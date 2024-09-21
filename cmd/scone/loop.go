package main

import (
	"fmt"
	"go/ast"

	"github.com/haijima/analysisutil"
	"github.com/haijima/analysisutil/ssautil"
	"github.com/haijima/scone/internal/analysis"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/tools/go/ssa"
)

func NewLoopCmd(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "loop"
	cmd.Aliases = []string{"loops", "n+1", "N+1"}
	cmd.Short = "Find N+1 queries"
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return runLoop(cmd, v)
	}

	return cmd
}

func runLoop(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	filter := v.GetString("filter")
	additionalFuncs := v.GetStringSlice("analyze-funcs")

	_, cgs, err := analysis.Analyze(cmd.Context(), dir, pattern, analysis.NewOption(filter, additionalFuncs))
	if err != nil {
		return err
	}

	pkgs, err := analysisutil.LoadPackages(dir, pattern)
	if err != nil {
		return err
	}

	positions := make([]string, 0)
	for _, pkg := range pkgs {
		ssaProg, err := ssautil.BuildSSA(pkg)
		if err != nil {
			return err
		}

		results := make([]FoundLoopedQuery, 0)
		for _, srcFunc := range ssaProg.SrcFuncs {
			results = append(results, analyzeForLoops(cgs, pkg.Syntax, srcFunc)...)
			for _, anonFunc := range srcFunc.AnonFuncs {
				results = append(results, analyzeForLoops(cgs, pkg.Syntax, anonFunc)...)
			}
		}
		for _, result := range results {
			positions = append(positions, fmt.Sprintf("%s, %s, %s", result.Func.Name(), result.Callee.Package().Pkg.Path()+"."+result.Callee.Name(), pkg.Fset.Position(result.Call.Pos())))
		}
	}

	//positions = slices.Compact(positions)
	if len(positions) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No N+1 queries found")
	} else if len(positions) == 1 {
		fmt.Fprintln(cmd.OutOrStdout(), "Found 1 N+1 query")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Found %d N+1 queries\n", len(positions))
	}
	for _, pos := range positions {
		fmt.Fprintln(cmd.OutOrStdout(), pos)
	}
	return nil
}

func analyzeForLoops(cgs map[string]*analysis.CallGraph, astFiles []*ast.File, fn *ssa.Function) []FoundLoopedQuery {
	results := make([]FoundLoopedQuery, 0)
	for _, astFile := range astFiles {
		ast.Inspect(astFile, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ForStmt:
				results = append(results, analyzeForLoopBody(cgs, fn, n.Body)...)
			case *ast.RangeStmt:
				results = append(results, analyzeForLoopBody(cgs, fn, n.Body)...)
			}
			return true
		})
	}
	return results
}

type FoundLoopedQuery struct {
	Func   *ssa.Function
	Callee *ssa.Function
	Call   *ssa.Call
}

func analyzeForLoopBody(cgs map[string]*analysis.CallGraph, fn *ssa.Function, body *ast.BlockStmt) []FoundLoopedQuery {
	results := make([]FoundLoopedQuery, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Pos() != 0 && body.Pos() <= instr.Pos() && instr.Pos() <= body.End() { // Check within the for loop
				if call, ok := instr.(*ssa.Call); ok {
					if callee := call.Call.StaticCallee(); callee != nil && callee.Pkg != nil {
						if callee.Pkg.Pkg.Path() == "database/sql" || callee.Pkg.Pkg.Path() == "github.com/jmoiron/sqlx" {
							results = append(results, FoundLoopedQuery{Func: fn, Callee: callee, Call: call})
						} else if cgs[callee.Pkg.Pkg.Path()] != nil && cgs[callee.Pkg.Pkg.Path()].Nodes[callee.Name()] != nil {
							results = append(results, FoundLoopedQuery{Func: fn, Callee: callee, Call: call})
						}
					}
				}
			}
		}
	}
	return results
}
