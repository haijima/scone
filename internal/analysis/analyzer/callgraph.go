package analyzer

import (
	"reflect"

	"github.com/haijima/scone/internal/analysis"
	toolsAnalysis "golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

const doc = "callgraph is ..."

// Analyzer is ...
var CallgraphAnalyzer = &toolsAnalysis.Analyzer{
	Name: "callgraph",
	Doc:  doc,
	Run: func(pass *toolsAnalysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		qgs := pass.ResultOf[QueryAnalyzer].(analysis.QueryResults)
		return analysis.BuildCallGraph(ssaProg.Pkg, qgs)
	},
	Requires: []*toolsAnalysis.Analyzer{
		buildssa.Analyzer,
		QueryAnalyzer,
	},
	ResultType: reflect.TypeOf(new(analysis.CallGraph)),
}
