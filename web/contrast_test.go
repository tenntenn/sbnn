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

// The foregrounds, and the surfaces they get painted on.
var (
	contrastForegrounds = []string{"--fg", "--fg-muted", "--accent", "--warn", "--danger"}
	contrastBackgrounds = []string{"--bg", "--bg-soft", "--bg-inset", "--selected", "--add-bg", "--del-bg"}
)

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
var contrastKnownLow = map[contrastPair]float64{
	// #116: --warn on the inset surface.
	{theme: "light", fg: "--warn", bg: "--bg-inset"}: 4.29,
	// #116: --warn on a deleted line.
	{theme: "light", fg: "--warn", bg: "--del-bg"}: 4.24,
	// #116: --danger on the selected-line highlight.
	{theme: "dark", fg: "--danger", bg: "--selected"}: 4.32,
}

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
	i := strings.Index(css, selector)
	if i < 0 {
		return "", false
	}
	rest := css[i+len(selector):]
	end := strings.Index(rest, "\n}")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}

// contrastTokens returns the colour tokens defined in a block.
func contrastTokens(block string) map[string]string {
	out := make(map[string]string)
	for _, m := range contrastDeclPattern.FindAllStringSubmatch(block, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
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
				// The web-tokens work is renaming and adding tokens in
				// parallel with this. A token this test does not find is
				// reported and skipped rather than failed: a legitimate
				// rename must not be blocked by a test that has not caught
				// up with it yet.
				fgv, hasFg := tokens[fg]
				if !hasFg {
					t.Logf("%s: %s is not defined; skipped", theme.name, fg)
					continue
				}
				bgv, hasBg := tokens[bg]
				if !hasBg {
					t.Logf("%s: %s is not defined; skipped", theme.name, bg)
					continue
				}
				ratio, ok := contrastRatio(fgv, bgv)
				if !ok {
					t.Logf("%s: %s (%s) on %s (%s) is not a plain hex colour; skipped",
						theme.name, fg, fgv, bg, bgv)
					continue
				}

				pair := contrastPair{theme: theme.name, fg: fg, bg: bg}
				seen[pair] = true
				got := contrastRound(ratio)
				known, isKnown := contrastKnownLow[pair]

				switch {
				case !isKnown && got < contrastMinAA:
					t.Errorf("%s: %s on %s is %.2f:1, below AA (%.1f:1)",
						theme.name, fg, bg, got, contrastMinAA)
				case isKnown && got >= contrastMinAA:
					t.Logf("%s: %s on %s is %.2f:1 and clears AA now "+
						"(was %.2f:1); it can come off contrastKnownLow",
						theme.name, fg, bg, got, known)
				case isKnown && got < known:
					t.Errorf("%s: %s on %s fell to %.2f:1, worse than the "+
						"known %.2f:1", theme.name, fg, bg, got, known)
				}
			}
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
	}
}
