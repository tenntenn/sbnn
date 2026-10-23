package selfupdate

import "testing"

func TestCanonical(t *testing.T) {
	for _, tt := range []struct {
		in, want string
	}{
		{"1.2.3", "v1.2.3"},
		{"v1.2.3", "v1.2.3"},
		{"0.0.1", "v0.0.1"},
		{"1.2.3-rc.1", "v1.2.3-rc.1"},
		{"v1.2", "v1.2.0"},
		{"dev", ""},
		{"", ""},
		{"latest", ""},
		{"1.2.3/../../x", ""},
	} {
		if got := Canonical(tt.in); got != tt.want {
			t.Errorf("Canonical(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewer(t *testing.T) {
	for _, tt := range []struct {
		latest, current string
		want            bool
	}{
		{"v0.0.2", "0.0.1", true},
		{"v0.0.1", "0.0.1", false},
		{"v0.0.1", "0.0.2", false},
		{"v0.10.0", "0.9.0", true}, // numeric, not lexical
		{"v1.0.0", "1.0.0-rc.1", true},
		{"v1.0.0-rc.2", "1.0.0-rc.1", true},
		{"v1.0.0-rc.1", "1.0.0", false},
		{"v0.9.0", "1.0.0-rc.1", false},
		{"v1.0.0", "dev", false}, // a source build is never told
		{"", "1.0.0", false},
		{"garbage", "1.0.0", false},
	} {
		if got := Newer(tt.latest, tt.current); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestCompare(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3", "1.2.4", -1},
		{"v2.0.0", "1.99.99", 1},
		{"v1.0.0-alpha", "1.0.0-beta", -1},
	} {
		if got := Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
