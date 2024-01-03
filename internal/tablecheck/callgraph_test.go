package tablecheck_test

import (
	"testing"

	"github.com/gostaticanalysis/testutil"
	"github.com/haijima/scone/internal/tablecheck"
	"golang.org/x/tools/go/analysis/analysistest"
)

// TestAnalyzer is a test for CallGraphAnalyzer.
func TestAnalyzer(t *testing.T) {
	testdata := testutil.WithModules(t, analysistest.TestData(), nil)
	analysistest.Run(t, testdata, tablecheck.CallGraphAnalyzer, "isucon13")
}
