package web

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// Contrast is arithmetic, so nothing here needs a browser or a person to
// look at it: every colour token is checked against every surface it can be
// painted on, and the ratio either clears WCAG AA or it does not.
//
// Every top level name here starts with "contrast" on purpose: tokens_test.go
// lives in this same package and is merged separately, so the two files must
// not collide over a shared helper name.

// contrastStylesheet is the stylesheet under test, relative to this package.
const contrastStylesheet = "src/styles.css"

// contrastMinAA is the WCAG 2.x AA ratio for text at normal size.
const contrastMinAA = 4.5

// The text colours, and the surfaces they get painted on. Every combination
// of the two is measured: any of these colours can end up on any of these
// surfaces, because the surface is chosen by where a row sits (hovered,
// selected, added, deleted) and the text colour by what the row says.
var (
	contrastForegrounds = []string{
		"--fg", "--fg-muted", "--accent-fg", "--warn-fg", "--danger-fg", "--ok-fg",
	}
	contrastBackgrounds = []string{
		"--bg", "--bg-soft", "--bg-inset", "--bg-elevated",
		"--surface-hover", "--surface-selected",
		"--add-bg", "--del-bg", "--accent-subtle",
	}
)

// contrastFills are the pairs where a colour is a fill and the text on it is
// fixed, so the cross product above does not describe them: white on the
// accent fill is a pair, white on the page is not. This is the case #122
// calls out separately -- a token can clear AA as text and miss it as a
// fill, which is why the two are now separate tokens.
var contrastFills = []struct{ fg, bg string }{
	{"--accent-on-fill", "--accent-fill"},
	{"--ok-fg", "--ok-bg"},
	{"--warn-fg", "--warn-bg"},
	{"--danger-fg", "--danger-bg"},
	{"--status-added-fg", "--status-added-bg"},
	{"--status-removed-fg", "--status-removed-bg"},
	{"--status-modified-fg", "--status-modified-bg"},
	{"--status-renamed-fg", "--status-renamed-bg"},
}

// contrastTheme names a block of token definitions in the stylesheet.
type contrastTheme struct {
	name     string
	selector string
}

var contrastThemes = []contrastTheme{
	{name: "light", selector: ":root {"},
	{name: "dark", selector: ":root[data-theme='dark'] {"},
}

// The @media (prefers-color-scheme: dark) block repeats the dark values
// verbatim, so reading the two blocks above covers every token; a drift
// between the two is caught by TestContrastDarkBlocksAgree.

// contrastPair is one foreground on one background in one theme.
type contrastPair struct {
	theme string
	fg    string
	bg    string
}

// contrastKnownLow records the pairs that are below AA today, with the ratio
// each had when this table was written.
//
// The point of the table is direction, not absolution: a pair listed here
// may not get worse, a pair not listed here may not drop below AA at all,
// and a pair that climbs back over AA is reported as an improvement rather
// than a failure. That way the fix lands without having to edit this file
// in the same change.
//
// Every entry here is an argument for #116, which proposes splitting the
// tokens into separate text and fill colours: the same token clears AA on
// one surface and misses it on another because it is being asked to do two
// jobs.
//
// It is empty today. The entries it held were the three pairs #116 was
// filed for, and the token split that issue asked for has landed: the
// colour that used to be text and fill at once is now --accent-fg and
// --accent-fill, and every pair measured below clears AA. An entry is added
// back only for a pair that is genuinely below AA and cannot be fixed in
// the same change.
var contrastKnownLow = map[contrastPair]float64{}

var contrastDeclPattern = regexp.MustCompile(`(?m)^\s*(--[A-Za-z0-9_-]+)\s*:\s*([^;]+);`)

// contrastReadCSS returns the stylesheet source.
func contrastReadCSS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(contrastStylesheet)
	if err != nil {
		t.Fatalf("read %s: %v", contrastStylesheet, err)
	}
	return string(b)
}

// contrastBlock returns the body of the rule introduced by selector: from
// the selector to the first closing brace in the first column.
func contrastBlock(css, selector string) (string, bool) {
	_, rest, ok := strings.Cut(css, selector)
	if !ok {
		return "", false
	}
	body, _, ok := strings.Cut(rest, "\n}")
	if !ok {
		return "", false
	}
	return body, true
}

// contrastTokens returns the colour tokens defined in a block, with each
// value folded to its canonical form by contrastCanonical.
func contrastTokens(block string) map[string]string {
	out := make(map[string]string)
	for _, m := range contrastDeclPattern.FindAllStringSubmatch(block, -1) {
		out[m[1]] = contrastCanonical(m[2])
	}
	return out
}

// contrastCanonical folds a declaration value to the form the browser reads.
// A value long enough to wrap -- --shadow-overlay is two shadows -- is
// written across several lines, and how far its continuation lines are
// indented is a matter of where in the file the block sits. CSS treats any
// run of whitespace as one space, so the tests do too: otherwise reindenting
// a block, which changes nothing anyone can see, reads as a changed value.
func contrastCanonical(v string) string {
	return strings.Join(strings.Fields(v), " ")
}

// contrastParseHex turns #rgb or #rrggbb into three channels in 0..1.
func contrastParseHex(v string) (r, g, b float64, ok bool) {
	v = strings.TrimSpace(v)
	if !strings.HasPrefix(v, "#") {
		return 0, 0, 0, false
	}
	h := v[1:]
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return 0, 0, 0, false
	}
	n, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return float64(n>>16&0xff) / 255, float64(n>>8&0xff) / 255, float64(n&0xff) / 255, true
}

// contrastChannel linearises one sRGB channel (WCAG 2.x).
func contrastChannel(c float64) float64 {
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

// contrastLuminance is the WCAG relative luminance of a colour.
func contrastLuminance(hex string) (float64, bool) {
	r, g, b, ok := contrastParseHex(hex)
	if !ok {
		return 0, false
	}
	return 0.2126*contrastChannel(r) + 0.7152*contrastChannel(g) + 0.0722*contrastChannel(b), true
}

// contrastRatio is the WCAG contrast ratio between two colours.
func contrastRatio(fg, bg string) (float64, bool) {
	lf, ok := contrastLuminance(fg)
	if !ok {
		return 0, false
	}
	lb, ok := contrastLuminance(bg)
	if !ok {
		return 0, false
	}
	hi, lo := lf, lb
	if hi < lo {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05), true
}

// contrastRound is the ratio to two decimals, which is the precision the
// known table is written to.
func contrastRound(v float64) float64 {
	return math.Round(v*100) / 100
}

// contrastCheck measures one foreground on one background and reports what
// the ratio means for the pair.
//
// A token that is not defined is a failure, not a skip. These names are the
// whole subject of the test: if a rename can quietly take a token out of the
// matrix, the matrix stops measuring anything and still passes -- which is
// how this file came to be checking two of its five foregrounds. Renaming a
// token means renaming it here too, and the failure says so.
func contrastCheck(t *testing.T, theme string, tokens map[string]string, fg, bg string, seen map[contrastPair]bool) {
	t.Helper()

	fgv, ok := tokens[fg]
	if !ok {
		t.Errorf("%s: %s is not defined in %s", theme, fg, contrastStylesheet)
		return
	}
	bgv, ok := tokens[bg]
	if !ok {
		t.Errorf("%s: %s is not defined in %s", theme, bg, contrastStylesheet)
		return
	}
	ratio, ok := contrastRatio(fgv, bgv)
	if !ok {
		// Not every surface is a flat hex: a translucent overlay has no
		// single ratio without knowing what is behind it.
		t.Logf("%s: %s (%s) on %s (%s) is not a plain hex colour; skipped",
			theme, fg, fgv, bg, bgv)
		return
	}

	pair := contrastPair{theme: theme, fg: fg, bg: bg}
	seen[pair] = true
	got := contrastRound(ratio)
	known, isKnown := contrastKnownLow[pair]

	switch {
	case !isKnown && got < contrastMinAA:
		t.Errorf("%s: %s on %s is %.2f:1, below AA (%.1f:1)",
			theme, fg, bg, got, contrastMinAA)
	case isKnown && got >= contrastMinAA:
		t.Logf("%s: %s on %s is %.2f:1 and clears AA now "+
			"(was %.2f:1); it can come off contrastKnownLow",
			theme, fg, bg, got, known)
	case isKnown && got < known:
		t.Errorf("%s: %s on %s fell to %.2f:1, worse than the "+
			"known %.2f:1", theme, fg, bg, got, known)
	}
}

func TestContrastTokens(t *testing.T) {
	css := contrastReadCSS(t)
	seen := make(map[contrastPair]bool)

	for _, theme := range contrastThemes {
		block, ok := contrastBlock(css, theme.selector)
		if !ok {
			t.Fatalf("no %q block in %s", theme.selector, contrastStylesheet)
		}
		tokens := contrastTokens(block)

		for _, fg := range contrastForegrounds {
			for _, bg := range contrastBackgrounds {
				contrastCheck(t, theme.name, tokens, fg, bg, seen)
			}
		}
		for _, fill := range contrastFills {
			contrastCheck(t, theme.name, tokens, fill.fg, fill.bg, seen)
		}
	}

	// A stale entry would otherwise sit here forever asserting nothing.
	for pair := range contrastKnownLow {
		if !seen[pair] {
			t.Logf("contrastKnownLow has %s %s on %s, which was not measured; "+
				"it can come off the table", pair.theme, pair.fg, pair.bg)
		}
	}
}

// TestContrastDarkBlocksAgree checks the two places the dark theme is
// written down. The @media block serves hosts that never stamp data-theme,
// the attribute block serves those that do, and a value that changes in one
// but not the other is a difference nobody sees until they switch hosts.
func TestContrastDarkBlocksAgree(t *testing.T) {
	css := contrastReadCSS(t)

	attr, ok := contrastBlock(css, ":root[data-theme='dark'] {")
	if !ok {
		t.Fatalf("no :root[data-theme='dark'] block in %s", contrastStylesheet)
	}
	media, ok := contrastBlock(css, ":root:not([data-theme='light']) {")
	if !ok {
		t.Fatalf("no :root:not([data-theme='light']) block in %s", contrastStylesheet)
	}

	attrTokens := contrastTokens(attr)
	mediaTokens := contrastTokens(media)

	for name, want := range attrTokens {
		got, ok := mediaTokens[name]
		if !ok {
			t.Errorf("%s is set for [data-theme='dark'] but not in the "+
				"prefers-color-scheme block", name)
			continue
		}
		if got != want {
			t.Errorf("%s is %s for [data-theme='dark'] but %s in the "+
				"prefers-color-scheme block", name, want, got)
		}
	}
	for name := range mediaTokens {
		if _, ok := attrTokens[name]; !ok {
			t.Errorf("%s is set in the prefers-color-scheme block but not "+
				"for [data-theme='dark']", name)
		}
	}
}

// contrastReport prints the whole table. It asserts nothing; it is here so
// that `go test -run Contrast -v` shows the numbers a change moved.
func TestContrastReport(t *testing.T) {
	css := contrastReadCSS(t)
	for _, theme := range contrastThemes {
		block, ok := contrastBlock(css, theme.selector)
		if !ok {
			continue
		}
		tokens := contrastTokens(block)
		for _, fg := range contrastForegrounds {
			row := make([]string, 0, len(contrastBackgrounds))
			for _, bg := range contrastBackgrounds {
				ratio, ok := contrastRatio(tokens[fg], tokens[bg])
				if !ok {
					continue
				}
				row = append(row, fmt.Sprintf("%s %.2f", strings.TrimPrefix(bg, "--"), ratio))
			}
			if len(row) > 0 {
				t.Logf("%-5s %-10s: %s", theme.name, fg, strings.Join(row, "  "))
			}
		}
		for _, fill := range contrastFills {
			ratio, ok := contrastRatio(tokens[fill.fg], tokens[fill.bg])
			if !ok {
				continue
			}
			t.Logf("%-5s %-10s: on %s %.2f", theme.name, fill.fg,
				strings.TrimPrefix(fill.bg, "--"), ratio)
		}
	}
}
