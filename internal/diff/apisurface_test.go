package diff

import (
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This test is not a freeze.
//
// It does not forbid changing what internal/diff exports; it makes changing
// it a deliberate act. #128 asks whether this package should be published
// as a library, and the honest answer needs the current surface to be
// something anyone can read off a page instead of reconstructing by grep.
// Until that question is settled, the surface is what a future v1 would be
// promising, so it should not grow by accident: an identifier that becomes
// exported because a helper was capitalised without thinking is exactly the
// kind of thing this catches.
//
// Adding to the list is a normal part of adding an exported identifier.
// Being unable to say why the list changed is the signal.

// apiSurfaceExported is every exported top-level identifier of this package.
//
// Note that it holds eleven entries, not the nine functions #128 lists: the
// two exported constants are part of the surface too, and GapMarker in
// particular is a printf format that anything reconstructing a file would
// have to match.
var apiSurfaceExported = []string{
	"const GapMarker",
	"const UnnamedPath",
	"func GeneratedMarker",
	"func ImageContentType",
	"func IsImage",
	"func IsMarkdown",
	"func IsNotebook",
	"func Parse",
	"func Reconstruct",
	"func Snippet",
	"func VisibleTop",
}

func TestAPISurface(t *testing.T) {
	got, err := apiSurfaceOf(".")
	if err != nil {
		t.Fatalf("reading the package: %v", err)
	}
	want := append([]string(nil), apiSurfaceExported...)
	sort.Strings(want)

	if diff := apiSurfaceDiff(want, got); diff != "" {
		t.Errorf("the exported surface of internal/diff is not what apiSurfaceExported says:\n%s\n\n"+
			"If you changed the exported surface, update the table above to match. "+
			"If you did not mean to change it, this is an unintended export.", diff)
	}
}

// apiSurfaceOf returns the exported top-level identifiers declared by the
// non-test Go files of a directory, sorted. Nothing is executed and no
// command is run: the files are read and parsed here.
func apiSurfaceOf(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	fset := gotoken.NewFileSet()
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := goparser.ParseFile(fset, filepath.Join(dir, name), nil, goparser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		out = append(out, apiSurfaceOfFile(f)...)
	}
	sort.Strings(out)
	return out, nil
}

// apiSurfaceOfFile returns the exported top-level identifiers of one file.
// A method is reported as "method Recv.Name" so that a new method on an
// exported type is visible here too.
func apiSurfaceOfFile(f *goast.File) []string {
	var out []string
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *goast.FuncDecl:
			if !goast.IsExported(d.Name.Name) {
				continue
			}
			if d.Recv == nil {
				out = append(out, "func "+d.Name.Name)
				continue
			}
			if recv := apiSurfaceReceiver(d.Recv); recv != "" {
				out = append(out, "method "+recv+"."+d.Name.Name)
			}
		case *goast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *goast.TypeSpec:
					if goast.IsExported(s.Name.Name) {
						out = append(out, "type "+s.Name.Name)
					}
				case *goast.ValueSpec:
					for _, n := range s.Names {
						if goast.IsExported(n.Name) {
							out = append(out, d.Tok.String()+" "+n.Name)
						}
					}
				}
			}
		}
	}
	return out
}

// apiSurfaceReceiver names the type a method is declared on, or "" when
// that type is unexported and the method is therefore not reachable.
func apiSurfaceReceiver(recv *goast.FieldList) string {
	if len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if star, ok := expr.(*goast.StarExpr); ok {
		expr = star.X
	}
	if idx, ok := expr.(*goast.IndexExpr); ok { // a generic receiver, Type[T]
		expr = idx.X
	}
	ident, ok := expr.(*goast.Ident)
	if !ok || !goast.IsExported(ident.Name) {
		return ""
	}
	return ident.Name
}

// apiSurfaceDiff describes how got differs from want, or "" when they are
// equal. Both must be sorted.
func apiSurfaceDiff(want, got []string) string {
	inWant := make(map[string]bool, len(want))
	for _, s := range want {
		inWant[s] = true
	}
	inGot := make(map[string]bool, len(got))
	for _, s := range got {
		inGot[s] = true
	}
	var b strings.Builder
	for _, s := range got {
		if !inWant[s] {
			fmt.Fprintf(&b, "  + %s (exported, but not in the table)\n", s)
		}
	}
	for _, s := range want {
		if !inGot[s] {
			fmt.Fprintf(&b, "  - %s (in the table, but no longer exported)\n", s)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
