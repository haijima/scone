package internal

import (
	"fmt"
	"strings"

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
	cg := static.CallGraph(ssaProg.Pkg.Prog)
	fnKindTableMemo := make(map[string]bool)
	for _, q := range foundQueries {
		k := fmt.Sprintf("%s#%s#%s", q.fn.Name(), q.kind, q.tables[0])
		if fnKindTableMemo[k] {
			continue
		}
		fnKindTableMemo[k] = true
		if q.kind == "SELECT" {
			fmt.Printf("\t\"%s\" -> \"%s\"[style=dotted];\n", q.fn.Name(), q.tables[0])
			//continue
		} else if q.kind == "INSERT" {
			fmt.Printf("\t\"%s\" -> \"%s\"[style=solid, color=green];\n", q.fn.Name(), q.tables[0])
		} else if q.kind == "UPDATE" {
			fmt.Printf("\t\"%s\" -> \"%s\"[style=bold, color=orange];\n", q.fn.Name(), q.tables[0])
		} else if q.kind == "DELETE" {
			fmt.Printf("\t\"%s\" -> \"%s\"[style=bold, color=red];\n", q.fn.Name(), q.tables[0])
		}
		callerFuncs = append(callerFuncs, q.fn)
	}

	done := make(map[*ssa.Function]bool)
	for len(callerFuncs) > 0 {
		fn := callerFuncs[0]
		callerFuncs = callerFuncs[1:]
		if done[fn] {
			continue
		}
		done[fn] = true
		if node, ok := cg.Nodes[fn]; ok {
			for _, edge := range node.In {
				caller := edge.Caller.Func
				callee := edge.Callee.Func
				fmt.Printf("\t\"%s\" -> \"%s\"[style=dashed];\n", caller.Name(), callee.Name())
				callerFuncs = append(callerFuncs, caller)
			}

			if len(node.In) == 0 {
				fmt.Printf("\t{rank = min; \"%s\"}\n", fn.Name())
			}
		}
	}

	// table node position
	fmt.Printf("\t{rank = max; %s}\n", strings.Join(q.tables, "; "))

	// table node style
	for _, table := range q.tables {
		kindMap := make(map[string]bool)
		for _, qq := range q.queriesByTable[table] {
			kindMap[qq.kind] = true
		}
		if kindMap["DELETE"] {
			fmt.Printf("\t\"%s\"[shape=box, style=bold, color=red, fontsize=\"21\", pad=1];\n", table)
		} else if kindMap["UPDATE"] {
			fmt.Printf("\t\"%s\"[shape=box, style=bold, color=orange, fontsize=\"21\", pad=1];\n", table)
		} else if kindMap["INSERT"] {
			fmt.Printf("\t\"%s\"[shape=box, style=solid, color=green, fontsize=\"21\", pad=1];\n", table)
		} else if kindMap["SELECT"] {
			fmt.Printf("\t\"%s\"[shape=box, style=dashed, fontsize=\"21\", pad=1];\n", table)
		}
	}

	fmt.Println("}")
	return nil, nil
}
