package tablecheck

import (
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/ssa"
)

const doc = "callgraph is ..."

// CallGraphAnalyzer is ...
var CallGraphAnalyzer = &analysis.Analyzer{
	Name: "callgraph",
	Doc:  doc,
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		q := pass.ResultOf[ExtractQueryAnalyzer].(*QueryResult)
		return CallGraph(ssaProg, q, defaultCallGraphOption)
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
		ExtractQueryAnalyzer,
	},
	ResultType: reflect.TypeOf(new(CallGraphResult)),
}

type CallGraphResult struct {
	Nodes map[string]*Node
}

func (r *CallGraphResult) Add(caller, callee *Node, edge *Edge) {
	if _, ok := r.Nodes[caller.Name]; !ok {
		r.Nodes[caller.Name] = caller
	}
	if _, ok := r.Nodes[callee.Name]; !ok {
		r.Nodes[callee.Name] = callee
	}

	edge.Caller = caller.Name
	edge.Callee = callee.Name

	r.Nodes[caller.Name].Out = append(r.Nodes[caller.Name].Out, edge)
	r.Nodes[callee.Name].In = append(r.Nodes[callee.Name].In, edge)
}

func TopologicalSort(nodes map[string]*Node) []*Node {
	visited := make(map[*Node]bool)
	sorted := make([]*Node, 0, len(nodes))
	var visit func(*Node)
	visit = func(node *Node) {
		if visited[node] {
			return
		}
		visited[node] = true
		for _, edge := range node.Out {
			visit(nodes[edge.Callee])
		}
		sorted = append(sorted, node)
	}
	for _, node := range nodes {
		visit(node)
	}
	return sorted
}

type Node struct {
	Name string
	In   []*Edge
	Out  []*Edge
	Func *ssa.Function
}

type Edge struct {
	SqlValue *SqlValue
	Caller   string
	Callee   string
}

type SqlValue struct {
	Kind   QueryKind
	RawSQL string
}

type CallGraphOption struct {
	IgnoreSelect bool
}

var defaultCallGraphOption = CallGraphOption{
	IgnoreSelect: false,
}

func CallGraph(ssaProg *buildssa.SSA, q *QueryResult, opt CallGraphOption) (*CallGraphResult, error) {
	result := &CallGraphResult{
		Nodes: make(map[string]*Node),
	}
	foundQueries := q.queries
	cg := static.CallGraph(ssaProg.Pkg.Prog)
	callerFuncs := make([]*ssa.Function, 0, len(foundQueries))
	queryEdgeMemo := make(map[string]bool)
	for _, q := range foundQueries {
		for _, t := range q.tables {
			if q.kind == Select && opt.IgnoreSelect {
				if _, ok := result.Nodes[t]; !ok {
					result.Nodes[t] = &Node{Name: t}
				}
				continue
			}

			k := fmt.Sprintf("%s#%s#%s", q.fn.Name(), q.kind, t)
			if queryEdgeMemo[k] {
				continue
			}
			queryEdgeMemo[k] = true

			if q.fn.Name() == "main" || q.fn.Name() == "init" {
				continue
			}
			if strings.HasPrefix(q.fn.Name(), "initializeHandler") {
				continue
			}
			result.Add(&Node{Name: q.fn.Name(), Func: q.fn}, &Node{Name: t}, &Edge{SqlValue: &SqlValue{Kind: q.kind, RawSQL: q.raw}})

			callerFuncs = append(callerFuncs, q.fn)
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
				result.Add(&Node{Name: caller.Name(), Func: caller}, &Node{Name: fn.Name(), Func: fn}, &Edge{})

				callerFuncs = append(callerFuncs, caller)
			}
		}
	}
	return result, nil
}
