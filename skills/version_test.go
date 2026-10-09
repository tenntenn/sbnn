package skills

import (
	"strings"
	"testing"
)

const sample = "---\nname: sbnn\ndescription: x\nlicense: MIT\n---\n\n# body\n"

func TestEmbeddedSkillCarriesNoVersion(t *testing.T) {
	md, err := Markdown()
	if err != nil {
		t.Fatal(err)
	}
	if v := Version(md); v != "" {
		t.Errorf("SKILL.md in the tree says version %q; the tag is the only authority, so the number is stamped at output time", v)
	}
}

func TestStamp(t *testing.T) {
	cases := []struct {
		name    string
		version string
		stamped bool
	}{
		{"a release is recorded", "1.2.3", true},
		{"a prerelease is recorded", "1.2.3-rc.1", true},
		{"dev records nothing", "dev", false},
		{"an empty version records nothing", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := Stamp([]byte(sample), tt.version)
			if !tt.stamped {
				if string(got) != sample {
					t.Errorf("Stamp() changed the file:\n%s", got)
				}
				return
			}
			if v := Version(got); v != tt.version {
				t.Errorf("Version(Stamp()) = %q, want %q", v, tt.version)
			}
			if !strings.HasSuffix(string(got), "---\n\n# body\n") {
				t.Errorf("the body was touched:\n%s", got)
			}
			if again := Stamp(got, tt.version); string(again) != string(got) {
				t.Errorf("Stamp is not idempotent:\n%s", again)
			}
		})
	}
}

func TestDecide(t *testing.T) {
	v100 := string(Stamp([]byte(sample), "1.0.0"))
	v110 := string(Stamp([]byte(sample+"more\n"), "1.1.0"))
	rc := string(Stamp([]byte(sample+"x\n"), "1.0.0-rc.1"))
	cases := []struct {
		name      string
		installed string
		own       string
		binary    string
		want      Action
	}{
		{"same text is current", v100, v100, "1.0.0", Current},
		{"a newer binary refreshes", v100, v110, "1.1.0", Refresh},
		{"an older binary leaves a newer skill", v110, v100, "1.0.0", Newer},
		{"a prerelease is older than its release", v100, rc, "1.0.0-rc.1", Newer},
		{"a dev build does not overwrite a released skill", v100, sample + "dev\n", "dev", Newer},
		{"an unstamped skill is refreshed by a release", sample, v100, "1.0.0", Refresh},
		{"an unstamped skill is refreshed by dev", sample, sample + "dev\n", "dev", Refresh},
		{"the same release with other text is refreshed", v100 + "edited\n", v100, "1.0.0", Refresh},
		{"a v prefix does not matter", v100, string(Stamp([]byte(sample+"x\n"), "v1.2.0")), "v1.2.0", Refresh},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := Decide([]byte(tt.installed), []byte(tt.own), tt.binary); got != tt.want {
				t.Errorf("Decide() = %v, want %v", got, tt.want)
			}
		})
	}
}
