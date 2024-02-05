package analyzer

import (
	"reflect"

	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/query"
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
		qgs := pass.ResultOf[QueryAnalyzer].([]*query.QueryGroup)
		return analysis.BuildCallGraph(ssaProg, qgs)
	},
	Requires: []*toolsAnalysis.Analyzer{
		buildssa.Analyzer,
		QueryAnalyzer,
	},
	ResultType: reflect.TypeOf(new(analysis.CallGraph)),
}
