package nogit_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tenntenn/sbnn/internal/analysis/nogit"
)

func TestAnalyzer(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), nogit.Analyzer, "a")
}
