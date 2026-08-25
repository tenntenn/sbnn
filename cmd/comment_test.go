package cmd

import "testing"

func TestNormalizeSide(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		in      string
		want    string
		wantErr bool
	}{
		"empty":         {in: "", want: "new"},
		"new":           {in: "new", want: "new"},
		"old":           {in: "old", want: "old"},
		"capitalized":   {in: "New", want: "new"},
		"upper":         {in: "OLD", want: "old"},
		"mixed":         {in: "nEw", want: "new"},
		"padded":        {in: " old ", want: "old"},
		"padded empty":  {in: "   ", want: "new"},
		"padded tab":    {in: "\tNEW\n", want: "new"},
		"unknown":       {in: "left", wantErr: true},
		"unknown upper": {in: "BOTH", wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeSide(test.in)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeSide(%q) = %q, want an error", test.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeSide(%q) returned an error: %v", test.in, err)
			}
			if got != test.want {
				t.Errorf("normalizeSide(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}
