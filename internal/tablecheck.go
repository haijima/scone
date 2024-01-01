package internal

import (
	"fmt"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/ssa"
)

const doc = "tablecheck is ..."

// Analyzer is ...
var Analyzer = &analysis.Analyzer{
	Name: "tablecheck",
	Doc:  doc,
	Run:  run,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
		ExtractQueryAnalyzer,
	},
}

func run(pass *analysis.Pass) (any, error) {
	ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	q := pass.ResultOf[ExtractQueryAnalyzer].(*queryResult)
	foundQueries := q.queries

	fmt.Println("digraph {")
	fmt.Println("\trankdir=\"LR\"")

	callerFuncs := make([]*ssa.Function, 0)
	calledFunc := make(map[*ssa.Function]bool)
	cg := static.CallGraph(ssaProg.Pkg.Prog)
	for _, q := range foundQueries {
		if q == nil {
			continue
		}
		if node, ok := cg.Nodes[q.fn]; ok {
			if q.kind == "SELECT" {
				continue
				fmt.Printf("\t\"%s\" -> \"%s\"[weight=100, style=dotted];\n", q.fn.Name(), q.tables[0])
			} else if q.kind == "INSERT" {
				fmt.Printf("\t\"%s\" -> \"%s\"[weight=100, style=solid, color=green];\n", q.fn.Name(), q.tables[0])
			} else if q.kind == "UPDATE" {
				fmt.Printf("\t\"%s\" -> \"%s\"[weight=100, style=bold, color=orange];\n", q.fn.Name(), q.tables[0])
			} else if q.kind == "DELETE" {
				fmt.Printf("\t\"%s\" -> \"%s\"[weight=100, style=bold, color=red];\n", q.fn.Name(), q.tables[0])
			}
			if calledFunc[q.fn] {
				continue
			}
			calledFunc[q.fn] = true
			for _, edge := range node.In {
				caller := edge.Caller.Func
				callee := edge.Callee.Func
				fmt.Printf("\t\"%s\" -> \"%s\"[style=dashed];\n", caller.Name(), callee.Name())
				callerFuncs = append(callerFuncs, caller)
			}
		}
	}

	for len(callerFuncs) > 0 {
		fn := callerFuncs[0]
		callerFuncs = callerFuncs[1:]
		if calledFunc[fn] {
			continue
		}
		calledFunc[fn] = true
		if node, ok := cg.Nodes[fn]; ok {
			for _, edge := range node.In {
				caller := edge.Caller.Func
				callee := edge.Callee.Func
				fmt.Printf("\t\"%s\" -> \"%s\"[style=dashed];\n", caller.Name(), callee.Name())
				callerFuncs = append(callerFuncs, caller)
			}
		}
	}

	fmt.Print("\t{rank = max;")
	for _, q := range foundQueries {
		fmt.Printf(" \"%s\";", q.tables[0])
	}
	fmt.Println("}")

	for _, q := range foundQueries {
		fmt.Printf("\t\"%s\"[shape=box, style=dashed, fontsize=\"21\", pad=1];\n", q.tables[0])
	}
	for _, q := range foundQueries {
		if q.kind == "INSERT" {
			fmt.Printf("\t\"%s\"[shape=box, style=bold, color=green, fontsize=\"21\", pad=1];\n", q.tables[0])
		}
	}
	for _, q := range foundQueries {
		if q.kind == "UPDATE" {
			fmt.Printf("\t\"%s\"[shape=box, style=bold, color=orange, fontsize=\"21\", pad=1];\n", q.tables[0])
		}
	}
	for _, q := range foundQueries {
		if q.kind == "DELETE" {
			fmt.Printf("\t\"%s\"[shape=box, style=bold, color=red, fontsize=\"21\", pad=1];\n", q.tables[0])
		}
	}

	fmt.Println("}")

	return nil, nil
}
