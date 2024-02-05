package analyzer

import (
	"reflect"

	"github.com/haijima/scone/internal/analysis"
	toolsAnalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

// Analyzer is ...
var QueryAnalyzer = &toolsAnalysis.Analyzer{
	Name: "extractquery",
	Doc:  "scone is ...",
	Run: func(pass *toolsAnalysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		return analysis.ExtractQuery(ssaProg, pass.Files, &analysis.Option{})
	},
	Requires: []*toolsAnalysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf([]*analysis.QueryResult{}),
}
