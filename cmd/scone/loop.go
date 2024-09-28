package main

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/haijima/analysisutil"
	"github.com/haijima/analysisutil/ssautil"
	"github.com/haijima/scone/internal/analysis"
	"github.com/jedib0t/go-pretty/v6/table"
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

	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|html|simple}")

	return cmd
}

type FoundLoopedQuery struct {
	Func     *ssa.Function
	Callee   *ssa.Function
	Call     *ssa.Call
	Position token.Position
	N        int
}

func runLoop(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	filter := v.GetString("filter")
	additionalFuncs := v.GetStringSlice("analyze-funcs")
	format := v.GetString("format")

	if !slices.Contains([]string{"table", "md", "csv", "tsv", "html", "simple"}, format) {
		return errors.Newf("unknown format: %s", format)
	}

	_, cgs, err := analysis.Analyze(cmd.Context(), dir, pattern, analysis.NewOption(filter, additionalFuncs))
	if err != nil {
		return err
	}

	pkgs, err := analysisutil.LoadPackages(dir, pattern)
	if err != nil {
		return err
	}

	results := make([]*FoundLoopedQuery, 0)
	for _, pkg := range pkgs {
		ssaProg, err := ssautil.BuildSSA(pkg)
		if err != nil {
			return err
		}

		for _, srcFunc := range ssaProg.SrcFuncs {
			results = append(results, analyzeForLoops(cgs, pkg.Syntax, srcFunc)...)
			//for _, anonFunc := range srcFunc.AnonFuncs {
			//	results = append(results, analyzeForLoops(cgs, pkg.Syntax, anonFunc)...)
			//}
		}
		for _, result := range results {
			result.Position = pkg.Fset.Position(result.Call.Pos())
		}
	}

	slices.SortFunc(results, func(a, b *FoundLoopedQuery) int { return strings.Compare(a.Position.String(), b.Position.String()) })
	cloned := slices.Clone(results)
	compacted := slices.CompactFunc(results, func(a, b *FoundLoopedQuery) bool { return a.Position.String() == b.Position.String() })
	for _, result := range compacted {
		count := 0
		for _, r := range cloned {
			if r.Position.String() == result.Position.String() {
				count++
			}
		}
		result.N = count
	}

	t := table.NewWriter()
	t.SetOutputMirror(cmd.OutOrStdout())
	t.AppendHeader(table.Row{"#", "Function Name", "Callee", "N", "Position"})

	for i, res := range compacted {
		t.AppendRow(table.Row{i + 1, res.Func.Name(), res.Callee.Package().Pkg.Path() + "." + res.Callee.Name(), res.N, res.Position})
	}

	switch format {
	case "table":
		t.Render()
	case "md":
		t.RenderMarkdown()
	case "csv":
		t.RenderCSV()
	case "tsv":
		t.RenderTSV()
	case "html":
		t.RenderHTML()
	case "simple":
		t.Style().Options.DrawBorder = false
		t.Style().Options.SeparateHeader = false
		t.Style().Options.SeparateRows = false
		t.Style().Box.MiddleVertical = " "
		t.Render()
	}
	return nil
}

func analyzeForLoops(cgs map[string]*analysis.CallGraph, astFiles []*ast.File, fn *ssa.Function) []*FoundLoopedQuery {
	results := make([]*FoundLoopedQuery, 0)
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

func analyzeForLoopBody(cgs map[string]*analysis.CallGraph, fn *ssa.Function, body *ast.BlockStmt) []*FoundLoopedQuery {
	results := make([]*FoundLoopedQuery, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if instr.Pos() != 0 && body.Pos() <= instr.Pos() && instr.Pos() <= body.End() { // Check within the for loop
				if call, ok := instr.(*ssa.Call); ok {
					if callee := call.Call.StaticCallee(); callee != nil && callee.Pkg != nil {
						if callee.Pkg.Pkg.Path() == "database/sql" || callee.Pkg.Pkg.Path() == "github.com/jmoiron/sqlx" {
							results = append(results, &FoundLoopedQuery{Func: fn, Callee: callee, Call: call})
						} else if cgs[callee.Pkg.Pkg.Path()] != nil && cgs[callee.Pkg.Pkg.Path()].Nodes[callee.Name()] != nil {
							results = append(results, &FoundLoopedQuery{Func: fn, Callee: callee, Call: call})
						}
					}
				}
			}
		}
	}
	return results
}
