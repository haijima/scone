package callgraph

import (
	"go/types"

	"github.com/haijima/scone/internal/analysis/query"
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

func (n *Node) IsFunc() bool {
	return n.Func != nil
}

func (n *Node) IsTable() bool {
	return n.Func == nil
}

func (n *Node) IsRoot() bool {
	return len(n.In) == 0
}

func (n *Node) IsNotRoot() bool {
	return !n.IsRoot()
}

type Edge struct {
	SqlValue *SqlValue
	Caller   string
	Callee   string
}

func (e *Edge) IsFuncCall() bool {
	return e.SqlValue == nil
}

func (e *Edge) IsQuery() bool {
	return e.SqlValue != nil
}

type SqlValue struct {
	Kind   query.QueryKind
	RawSQL string
}

func Walk(cg *CallGraph, in *Node, fn func(edge *Edge) bool) {
	queue := []string{in.Name}
	for len(queue) > 0 {
		callee := queue[0]
		queue = queue[1:]
		if n, exists := cg.Nodes[callee]; exists {
			for _, e := range n.Out {
				if skip := fn(e); !skip {
					queue = append(queue, e.Callee)
				}
			}
		}
	}
}
