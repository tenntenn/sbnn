package asset_test

import (
	"testing"

	"github.com/tenntenn/sbnn/internal/asset"
)

// The page budget is spent across files, so what it answers depends on what
// came before. It is pinned at MaxPageBytes and MaxPageBytes+1, the way
// TestInDiffCapsAnImageOfTheDiffItself pins the per-file cap (#356).
func TestSpendBoundsThePage(t *testing.T) {
	cases := []struct {
		name       string
		status     asset.Status
		size       int64
		spent      int64
		wantStatus asset.Status
		wantSpent  int64
	}{
		{"first picture of an empty page", asset.StatusOK, 1024, 0, asset.StatusOK, 1024},
		{"fills the page exactly", asset.StatusOK, asset.MaxBytes, asset.MaxPageBytes - asset.MaxBytes, asset.StatusOK, asset.MaxPageBytes},
		{"one byte past the page", asset.StatusOK, asset.MaxBytes, asset.MaxPageBytes - asset.MaxBytes + 1, asset.StatusOverBudget, asset.MaxPageBytes - asset.MaxBytes + 1},
		{"page already full, one byte more", asset.StatusOK, 1, asset.MaxPageBytes, asset.StatusOverBudget, asset.MaxPageBytes},
		{"too large costs nothing", asset.StatusTooLarge, asset.MaxBytes + 1, 0, asset.StatusTooLarge, 0},
		{"missing costs nothing", asset.StatusMissing, 0, 5, asset.StatusMissing, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spent := c.spent
			got := asset.Spend(c.status, c.size, &spent)
			if got != c.wantStatus || spent != c.wantSpent {
				t.Errorf("Spend(%q, %d, spent=%d) = %q, spent=%d, want %q, spent=%d",
					c.status, c.size, c.spent, got, spent, c.wantStatus, c.wantSpent)
			}
		})
	}
}
