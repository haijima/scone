package callgraph

import (
	"go/types"

	"github.com/haijima/scone/internal/tablecheck/query"
	"golang.org/x/tools/go/ssa"
)

type CallGraph struct {
	Package *types.Package
	Nodes   map[string]*Node
}

func (r *CallGraph) AddNode(n *Node) {
	if _, ok := r.Nodes[n.Name]; !ok {
		r.Nodes[n.Name] = n
	}
}

func (r *CallGraph) AddFuncCallEdge(callerFunc, calleeFunc *ssa.Function) {
	r.add(&Node{Name: callerFunc.Name(), Func: callerFunc}, &Node{Name: calleeFunc.Name(), Func: calleeFunc}, &Edge{})
}

func (r *CallGraph) AddQueryEdge(callerFunc *ssa.Function, calleeTable string, sqlValue *SqlValue) {
	r.add(&Node{Name: callerFunc.Name(), Func: callerFunc}, &Node{Name: calleeTable}, &Edge{SqlValue: sqlValue})
}

func (r *CallGraph) add(caller, callee *Node, edge *Edge) {
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
	Kind   query.QueryKind
	RawSQL string
}
