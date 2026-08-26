package doccheck

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/tenntenn/sbnn/internal/asset"
	"github.com/tenntenn/sbnn/internal/history"
	"github.com/tenntenn/sbnn/internal/model"
	"github.com/tenntenn/sbnn/internal/server"
)

// docs/api.md is the reference three proposed clients would be written
// against (#125, #105, #147), and nothing but this file notices when it stops
// being true.
//
// The drift #148 points at already happened once, on the one client that was
// documented: the skill's description of model.Comment listed two omitempty
// fields as always present and missed five real ones (#110). That is a
// hand-written account of a Go struct getting out of step with the struct, so
// the check is against the structs themselves rather than against a copy of
// them.

func readAPIDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "api.md"))
	if err != nil {
		t.Fatalf("reading docs/api.md: %v", err)
	}
	return string(b)
}

// section returns the part of the document under the "#### `name`" heading,
// up to the next heading. A field is only documented if it appears under its
// own type: "path" turning up under some other struct proves nothing.
func section(t *testing.T, doc, name string) string {
	t.Helper()
	head := "#### `" + name + "`\n"
	i := strings.Index(doc, head)
	if i < 0 {
		t.Fatalf("docs/api.md has no %q section", head)
	}
	rest := doc[i+len(head):]
	if before, _, ok := strings.Cut(rest, "\n#"); ok {
		return before
	}
	return rest
}

// jsonNames is the wire name of every field of a struct, in declaration
// order, skipping the ones json never writes.
func jsonNames(t *testing.T, typ reflect.Type) []string {
	t.Helper()
	if typ.Kind() != reflect.Struct {
		t.Fatalf("%s is not a struct", typ)
	}
	var out []string
	for f := range typ.Fields() {
		if !f.IsExported() {
			continue
		}
		tag, ok := f.Tag.Lookup("json")
		if !ok {
			out = append(out, f.Name)
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// TestAPIDocCoversEveryPayloadField walks the types the API answers with and
// fails on a field that docs/api.md does not mention under that type.
func TestAPIDocCoversEveryPayloadField(t *testing.T) {
	doc := readAPIDoc(t)
	types := []struct {
		name string
		typ  reflect.Type
		// extra is what the wire carries that no struct field does.
		extra []string
	}{
		{name: "Status", typ: reflect.TypeFor[server.Status]()},
		{name: "GroupSummary", typ: reflect.TypeFor[server.GroupSummary]()},
		// Comment.MarshalJSON adds the suggestions parsed out of the body,
		// so the struct alone does not describe what a client receives.
		{name: "Comment", typ: reflect.TypeFor[model.Comment](), extra: []string{"suggestions"}},
		{name: "Group", typ: reflect.TypeFor[model.Group]()},
		{name: "Diff", typ: reflect.TypeFor[model.Diff]()},
		{name: "File", typ: reflect.TypeFor[model.File]()},
		{name: "Hunk", typ: reflect.TypeFor[model.Hunk]()},
		{name: "Line", typ: reflect.TypeFor[model.Line]()},
		{name: "Hook", typ: reflect.TypeFor[model.Hook]()},
		{name: "AddDiffRequest", typ: reflect.TypeFor[server.AddDiffRequest]()},
		{name: "AddDiffResponse", typ: reflect.TypeFor[server.AddDiffResponse]()},
		{name: "AddCommentRequest", typ: reflect.TypeFor[server.AddCommentRequest]()},
		{name: "UpdateCommentRequest", typ: reflect.TypeFor[server.UpdateCommentRequest]()},
		{name: "SubmitReviewRequest", typ: reflect.TypeFor[server.SubmitReviewRequest]()},
		{name: "PreviewResponse", typ: reflect.TypeFor[server.PreviewResponse]()},
		{name: "FileContentResponse", typ: reflect.TypeFor[server.FileContentResponse]()},
		{name: "Entry", typ: reflect.TypeFor[asset.Entry]()},
		{name: "ReviewsResponse", typ: reflect.TypeFor[server.ReviewsResponse]()},
		{name: "Record", typ: reflect.TypeFor[history.Record]()},
		{name: "history.Comment", typ: reflect.TypeFor[history.Comment]()},
		{name: "Stats", typ: reflect.TypeFor[history.Stats]()},
		{name: "Count", typ: reflect.TypeFor[history.Count]()},
	}
	for _, tc := range types {
		t.Run(tc.name, func(t *testing.T) {
			body := section(t, doc, tc.name)
			for _, name := range append(jsonNames(t, tc.typ), tc.extra...) {
				if !strings.Contains(body, "`"+name+"`") {
					t.Errorf("docs/api.md does not document %s.%s\n"+
						"add a row for `%s`, or drop the field", tc.name, name, name)
				}
			}
		})
	}
}

// routePattern reads the routes out of the server's own mux registrations, so
// a route added without a line in the reference is caught by the same change
// that added it.
var routePattern = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) (/_/[^"]*)"`)

func TestAPIDocCoversEveryRoute(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "internal", "server", "server.go"))
	if err != nil {
		t.Fatalf("reading internal/server/server.go: %v", err)
	}
	matches := routePattern.FindAllStringSubmatch(string(b), -1)
	if len(matches) == 0 {
		t.Fatal("found no routes in internal/server/server.go; has the mux moved?")
	}
	doc := readAPIDoc(t)
	seen := make(map[string]bool, len(matches))
	for _, m := range matches {
		route := m[1] + " " + m[2]
		if seen[route] {
			// Two patterns for one handler - .../hooks and .../hooks/{id}
			// are distinct routes, but an exact repeat would not be.
			continue
		}
		seen[route] = true
		t.Run(route, func(t *testing.T) {
			if !strings.Contains(doc, "### `"+route+"`") {
				t.Errorf("docs/api.md has no section for %s\nadd \"### `%s`\"", route, route)
			}
		})
	}
	// And nothing documented that no longer exists.
	documented := regexp.MustCompile("(?m)^### `([A-Z]+) (/_/[^`]*)`").FindAllStringSubmatch(doc, -1)
	for _, m := range documented {
		route := m[1] + " " + m[2]
		if !seen[route] {
			t.Errorf("docs/api.md documents %s, which the server does not serve", route)
		}
	}
}

// The rule a client author would otherwise meet as a 403 with no way to know
// it was expected. It is the reason the most dangerous call in the API - a
// hook is a shell command the server runs - cannot be made from another site.
func TestAPIDocDescribesTheCrossOriginRule(t *testing.T) {
	doc := readAPIDoc(t)
	for _, want := range []string{
		"Sec-Fetch-Site",
		"Origin",
		"sbnn only takes changes from its own page or from the command line",
		"403",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/api.md does not mention %q in the cross-origin rule", want)
		}
	}
}

// #148 asks for a decision, not silence: "unstable, here is the shape today"
// is a good answer and tells a client author what they are signing up for.
func TestAPIDocSaysWhetherItIsStable(t *testing.T) {
	doc := readAPIDoc(t)
	if !strings.Contains(doc, "not a stable interface") {
		t.Error("docs/api.md does not say whether /_/api/ is stable")
	}
}

// The replay on Last-Event-ID is a snapshot of where each group stands, not
// the backlog the header name suggests. A client author who assumes otherwise
// loses the intermediate reviews with nothing to notice: the ids that were
// skipped never arrive, so there is no gap in the sequence to see.
//
// That is the design (see TestReviewReplayIsOnePerGroupNotEveryNotice in
// internal/server), and #336 was filed because the document did not say so.
// The behaviour is held there; this holds the sentence that tells a reader.
func TestAPIDocSaysTheReplayIsPerGroup(t *testing.T) {
	doc := readAPIDoc(t)
	i := strings.Index(doc, "### `GET /_/events`")
	if i < 0 {
		t.Fatal("docs/api.md has no `GET /_/events` section")
	}
	events := doc[i:]
	if before, _, ok := strings.Cut(events[len("### `GET /_/events`"):], "\n## "); ok {
		events = before
	}
	// Markdown wraps, so a phrase can be split across two lines. Match on the
	// prose rather than on the line breaks.
	events = strings.Join(strings.Fields(events), " ")
	for _, want := range []string{
		"Last-Event-ID",
		"one stored notice per group",
		"snapshot, not a backlog",
	} {
		if !strings.Contains(events, want) {
			t.Errorf("the `GET /_/events` section of docs/api.md does not say %q;\n"+
				"a reader is left thinking Last-Event-ID replays every review after n", want)
		}
	}
}
