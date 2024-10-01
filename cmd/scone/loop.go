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
	"golang.org/x/tools/go/packages"
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
		bodies := getForRangeBodies(pkg.Syntax)
		results = append(results, analyzeForLoopBody(cgs, pkg, ssaProg.SrcFuncs, bodies)...)
	}

	slices.SortFunc(results, func(a, b *FoundLoopedQuery) int { return strings.Compare(a.Position.String(), b.Position.String()) })

	t := table.NewWriter()
	t.SetOutputMirror(cmd.OutOrStdout())
	t.AppendHeader(table.Row{"#", "Function Name", "Callee", "N", "Position"})

	for i, res := range results {
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

func getForRangeBodies(astFiles []*ast.File) []*ast.BlockStmt {
	bodies := make([]*ast.BlockStmt, 0)
	for _, astFile := range astFiles {
		ast.Inspect(astFile, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.ForStmt:
				bodies = append(bodies, n.Body)
			case *ast.RangeStmt:
				bodies = append(bodies, n.Body)
			}
			return true
		})
	}
	return bodies
}

func analyzeForLoopBody(cgs map[string]*analysis.CallGraph, pkg *packages.Package, fns []*ssa.Function, bodies []*ast.BlockStmt) []*FoundLoopedQuery {
	results := make([]*FoundLoopedQuery, 0)
	for _, fn := range fns {
		for _, block := range fn.Blocks {
			for _, instr := range block.Instrs {
				if n := withInForLoop(instr, bodies); n > 0 { // Check within the for loop
					if call, ok := instr.(*ssa.Call); ok {
						if callee := call.Call.StaticCallee(); callee != nil && callee.Pkg != nil {
							if callee.Pkg.Pkg.Path() == "database/sql" ||
								callee.Pkg.Pkg.Path() == "github.com/jmoiron/sqlx" ||
								(cgs[callee.Pkg.Pkg.Path()] != nil && cgs[callee.Pkg.Pkg.Path()].Nodes[callee.Name()] != nil) {
								results = append(results, &FoundLoopedQuery{Func: fn, Callee: callee, Call: call, Position: pkg.Fset.Position(call.Pos()), N: n})
							}
						}
					}
				}
			}
		}
	}
	return results
}

func withInForLoop(instr ssa.Instruction, bodies []*ast.BlockStmt) int {
	if instr.Pos() == 0 {
		return 0
	}
	var cnt int
	for _, body := range bodies {
		if body.Pos() <= instr.Pos() && instr.Pos() <= body.End() {
			cnt++
		}
	}
	return cnt
}
