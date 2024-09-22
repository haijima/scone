package analysis

import (
	"slices"

	"github.com/haijima/scone/internal/sql"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/ssa"
)

type CallGraph struct {
	Nodes map[string]*Node
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

	contains := slices.ContainsFunc(r.Nodes[caller.Name].Out, func(e *Edge) bool {
		if e.Callee != callee.Name {
			return false
		}
		return (e.IsFuncCall() && edge.IsFuncCall()) || (e.IsQuery() && edge.IsQuery() && e.SqlValue.Kind == edge.SqlValue.Kind)
	})
	if !contains {
		r.Nodes[caller.Name].Out = append(r.Nodes[caller.Name].Out, edge)
		r.Nodes[callee.Name].In = append(r.Nodes[callee.Name].In, edge)
	}
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
	Kind   sql.QueryKind
	RawSQL string
}

func Walk(cg *CallGraph, in *Node, fn func(node *Node) bool) {
	queue := []string{in.Name}
	seen := make(map[string]bool)
	for len(queue) > 0 {
		callee := queue[0]
		queue = queue[1:]
		if seen[callee] {
			continue
		}
		seen[callee] = true
		if n, exists := cg.Nodes[callee]; exists {
			if skip := fn(n); !skip {
				for _, e := range n.Out {
					queue = append(queue, e.Callee)
				}
			}
		}
	}
}

func BuildCallGraph(pkg *ssa.Package, qrs []*QueryResult) (*CallGraph, error) {
	result := &CallGraph{Nodes: make(map[string]*Node)}
	callerFuncs := make([]*ssa.Function, 0, len(qrs))
	for _, qr := range qrs {
		callerFuncs = append(callerFuncs, qr.Posx.Func)
		for _, q := range qr.Queries() {
			for _, t := range q.Tables {
				if q.MainTable == t {
					result.AddQueryEdge(qr.Posx.Func, t, &SqlValue{Kind: q.Kind, RawSQL: q.Raw})
				} else {
					result.AddQueryEdge(qr.Posx.Func, t, &SqlValue{Kind: sql.Select, RawSQL: q.Raw})
				}
			}
		}
	}

	cg := static.CallGraph(pkg.Prog)
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
