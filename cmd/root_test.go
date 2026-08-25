package cmd

import (
	"maps"
	"strings"
	"testing"
)

func TestParseLabels(t *testing.T) {
	tests := []struct {
		name    string
		flags   []string
		want    map[string]string
		wantErr string // a substring the error has to name
	}{
		{
			name:  "plain pair",
			flags: []string{"a=1"},
			want:  map[string]string{"a": "1"},
		},
		{
			name:  "empty value is kept",
			flags: []string{"a="},
			want:  map[string]string{"a": ""},
		},
		{
			name:  "spaces around the key and the value go",
			flags: []string{" a = 1 "},
			want:  map[string]string{"a": "1"},
		},
		{
			name:  "the value may hold an =",
			flags: []string{"a=b=c"},
			want:  map[string]string{"a": "b=c"},
		},
		{
			name:    "a repeated key is refused by name",
			flags:   []string{"a=1", "a=2"},
			wantErr: `"a"`,
		},
		{
			name:    "the duplicate is seen after trimming",
			flags:   []string{"a=1", " a =2"},
			wantErr: `"a"`,
		},
		{
			name:    "no key",
			flags:   []string{"=1"},
			wantErr: "wants key=value",
		},
		{
			name:    "no key once trimmed",
			flags:   []string{" = 1"},
			wantErr: "wants key=value",
		},
		{
			name:    "no separator",
			flags:   []string{"a"},
			wantErr: "wants key=value",
		},
		{
			name:  "no flags at all",
			flags: nil,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLabels(tt.flags)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("parseLabels(%q) = %v, nil; want an error", tt.flags, got)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("parseLabels(%q) error = %v; want it to mention %s", tt.flags, err, tt.wantErr)
				}
				if got != nil {
					t.Errorf("parseLabels(%q) = %v; want no labels alongside the error", tt.flags, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseLabels(%q): %v", tt.flags, err)
			}
			if !maps.Equal(got, tt.want) {
				t.Errorf("parseLabels(%q) = %v; want %v", tt.flags, got, tt.want)
			}
		})
	}
}

// TestParseLabelsDuplicateMentionsTheKey pins the wording: a script that hits
// this has to be told which key it repeated.
func TestParseLabelsDuplicateMentionsTheKey(t *testing.T) {
	_, err := parseLabels([]string{"pr=101", "pr=102"})
	if err == nil {
		t.Fatal("parseLabels: want an error for a repeated key")
	}
	if !strings.Contains(err.Error(), "pr") || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("parseLabels error = %v; want it to name pr and say it was given more than once", err)
	}
}
