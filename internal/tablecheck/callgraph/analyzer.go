package callgraph

import (
	"fmt"
	"reflect"

	"github.com/haijima/scone/internal/tablecheck/query"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/ssa"
)

const doc = "callgraph is ..."

// Analyzer is ...
var Analyzer = &analysis.Analyzer{
	Name: "callgraph",
	Doc:  doc,
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		q := pass.ResultOf[query.Analyzer].(*query.Result)
		return BuildCallGraph(ssaProg, q, defaultCallGraphOption)
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
		query.Analyzer,
	},
	ResultType: reflect.TypeOf(new(CallGraph)),
}

type CallGraphOption struct {
	IgnoreSelect bool
}

var defaultCallGraphOption = CallGraphOption{
	IgnoreSelect: false,
}

func BuildCallGraph(ssaProg *buildssa.SSA, q *query.Result, opt CallGraphOption) (*CallGraph, error) {
	result := &CallGraph{
		Nodes: make(map[string]*Node),
	}
	foundQueries := q.Queries
	cg := static.CallGraph(ssaProg.Pkg.Prog)
	callerFuncs := make([]*ssa.Function, 0, len(foundQueries))
	queryEdgeMemo := make(map[string]bool)
	for _, q := range foundQueries {
		for _, t := range q.Tables {
			if q.Kind == query.Select && opt.IgnoreSelect {
				result.AddNode(&Node{Name: t})
				continue
			}

			k := fmt.Sprintf("%s#%s#%s", q.Func.Name(), q.Kind, t)
			if queryEdgeMemo[k] {
				continue
			}
			queryEdgeMemo[k] = true

			if q.Func.Name() == "main" || q.Func.Name() == "init" {
				continue
			}
			result.AddQueryEdge(q.Func, t, &SqlValue{Kind: q.Kind, RawSQL: q.Raw})

			callerFuncs = append(callerFuncs, q.Func)
		}
	}

	seen := make(map[*ssa.Function]bool)
	for len(callerFuncs) > 0 {
		fn := callerFuncs[0]
		callerFuncs = callerFuncs[1:]
		if seen[fn] {
			continue
		}
		seen[fn] = true
		if node, ok := cg.Nodes[fn]; ok {
			for _, edge := range node.In {
				caller := edge.Caller.Func
				result.AddFuncCallEdge(caller, fn)

				callerFuncs = append(callerFuncs, caller)
			}
		}
	}
	return result, nil
}
