package selfupdate

import (
	"strings"

	"golang.org/x/mod/semver"
)

// Canonical returns v as package semver spells it ("v1.2.3", "v1.2.3-rc.1"),
// or "" when v is not a release version. Both "1.2.3", which is how
// GoReleaser stamps the binary, and "v1.2.3", which is how the tag is
// spelled, are accepted; "dev" and a pseudo-version-less build are not.
func Canonical(v string) string {
	if v == "" || v == "dev" {
		return ""
	}
	s := "v" + strings.TrimPrefix(v, "v")
	if !semver.IsValid(s) {
		return ""
	}
	return semver.Canonical(s)
}

// Compare compares two versions the way semver orders them, pre-releases
// included: v1.2.0-rc.1 comes before v1.2.0. A version that is not a release
// sorts before every one that is, which is never asked for in practice:
// callers check Canonical first.
func Compare(a, b string) int {
	return semver.Compare(Canonical(a), Canonical(b))
}

// Newer reports whether latest is a release newer than current. A current
// that is no release at all (a source build) has nothing to compare with,
// so it is never told about a newer one.
func Newer(latest, current string) bool {
	if Canonical(latest) == "" || Canonical(current) == "" {
		return false
	}
	return Compare(latest, current) > 0
}
