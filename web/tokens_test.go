package web

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// A design token that is referenced but never defined is a silent failure by
// construction. var(--ok-bg, #1a7f37) renders perfectly, looks deliberate,
// and hides that the token was never defined - so the colour ignores the
// theme and no reviewer reading the diff can see it. Nothing else in the
// project catches this, which is why it is a test rather than a convention.
//
// Every top level name here starts with "tokens" on purpose: contrast_test.go
// lives in this same package and is merged separately, so the two files must
// not collide over a shared helper name.

// tokensStylesheet is the stylesheet under test, relative to this package.
const tokensStylesheet = "src/styles.css"

var (
	// A reference: var(--name) or var(--name, fallback).
	tokensRefPattern = regexp.MustCompile(`var\(\s*(--[A-Za-z0-9_-]+)`)
	// A definition: --name: value, at the start of a declaration.
	tokensDefPattern = regexp.MustCompile(`(?m)^\s*(--[A-Za-z0-9_-]+)\s*:`)
)

// tokensUndefinedAllowed lists the custom properties that may be referenced
// without this stylesheet defining them.
//
// This is a permission, not a requirement: a property listed here that gains
// a definition does not fail the test. Two of these are known defects and
// are expected to gain one.
var tokensUndefinedAllowed = map[string]string{
	// Set as an inline style by src/components/DiffStack.tsx, which measures
	// the toolbar and publishes its height. Having no CSS definition is
	// correct - the element that knows the number sets it.
	"--diff-toolbar-h": "set inline by src/components/DiffStack.tsx",
	// Set as an inline style by src/components/PreviewStack.tsx, same way.
	"--preview-toolbar-h": "set inline by src/components/PreviewStack.tsx",
	// Known defect, issue #81: referenced with a fallback but defined
	// nowhere, so it silently ignores the theme. Allowed until #81 lands.
	"--bg-elevated": "known defect, issue #81",
	// Known defect, issue #81, same as --bg-elevated.
	"--ok-bg": "known defect, issue #81",
}

// tokensReadCSS returns the stylesheet source.
func tokensReadCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(tokensStylesheet)
	if err != nil {
		t.Fatalf("read %s: %v", tokensStylesheet, err)
	}
	return string(b)
}

// tokensReferenced returns every custom property the stylesheet reads.
func tokensReferenced(css string) map[string]bool {
	found := make(map[string]bool)
	for _, m := range tokensRefPattern.FindAllStringSubmatch(css, -1) {
		found[m[1]] = true
	}
	return found
}

// tokensDefined returns every custom property the stylesheet declares.
func tokensDefined(css string) map[string]bool {
	found := make(map[string]bool)
	for _, m := range tokensDefPattern.FindAllStringSubmatch(css, -1) {
		found[m[1]] = true
	}
	return found
}

func TestTokensAllDefined(t *testing.T) {
	css := tokensReadCSS(t)
	referenced := tokensReferenced(css)
	defined := tokensDefined(css)

	if len(referenced) == 0 {
		t.Fatalf("no var() references found in %s; the pattern is wrong", tokensStylesheet)
	}
	if len(defined) == 0 {
		t.Fatalf("no custom properties defined in %s; the pattern is wrong", tokensStylesheet)
	}

	var undefined []string
	for name := range referenced {
		if defined[name] {
			continue
		}
		if _, ok := tokensUndefinedAllowed[name]; ok {
			continue
		}
		undefined = append(undefined, name)
	}
	sort.Strings(undefined)

	for _, name := range undefined {
		t.Errorf("%s references %s but never defines it; "+
			"var() would fall back silently and ignore the theme", tokensStylesheet, name)
	}
}

// TestTokensAllowlistIsCurrent reports allowlist entries that have since been
// defined. It never fails: an entry that gains a definition is the fix
// landing, not a regression. It only says the entry can go.
func TestTokensAllowlistIsCurrent(t *testing.T) {
	css := tokensReadCSS(t)
	defined := tokensDefined(css)
	referenced := tokensReferenced(css)

	names := make([]string, 0, len(tokensUndefinedAllowed))
	for name := range tokensUndefinedAllowed {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		switch {
		case defined[name]:
			t.Logf("%s is defined now (%s); it can come off the allowlist",
				name, tokensUndefinedAllowed[name])
		case !referenced[name]:
			t.Logf("%s is no longer referenced (%s); it can come off the allowlist",
				name, tokensUndefinedAllowed[name])
		}
	}
}
