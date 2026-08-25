package cmd

import (
	"strings"
	"testing"
)

func TestValidateClearFlags(t *testing.T) {
	tests := []struct {
		name     string
		doClear  bool
		clearAll bool
		wantErr  bool
	}{
		{name: "--clear --all", doClear: true, clearAll: true},
		// The trap this check has to avoid: cobra's "required together"
		// pairing would reject this one, and it is the ordinary way to
		// close a single review.
		{name: "--clear on its own", doClear: true},
		{name: "--all on its own", clearAll: true, wantErr: true},
		{name: "neither flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateClearFlags(tt.doClear, tt.clearAll)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateClearFlags(%v, %v) = nil; want an error", tt.doClear, tt.clearAll)
				}
				if !strings.Contains(err.Error(), "--clear") {
					t.Errorf("validateClearFlags(%v, %v) error = %v; want it to point at --clear",
						tt.doClear, tt.clearAll, err)
				}
				return
			}
			if err != nil {
				t.Errorf("validateClearFlags(%v, %v) = %v; want no error", tt.doClear, tt.clearAll, err)
			}
		})
	}
}
