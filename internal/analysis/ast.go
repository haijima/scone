package analysis

import (
	"go/ast"
	"go/token"

	"github.com/haijima/scone/internal/sql"
	"golang.org/x/tools/go/ssa"
)

func AnalyzeFuncByAst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*QueryResult {
	foundQueryGroups := make([]*QueryResult, 0)
	ast.Inspect(fn.Syntax(), func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if q, ok := sql.ParseString(lit.Value); ok {
				meta := NewMeta(pkg, fn, lit.Pos(), pos...)
				if opt.Filter(q, meta) {
					foundQueryGroups = append(foundQueryGroups, &QueryResult{QueryGroup: sql.NewQueryGroupFrom(q), Meta: meta})
				}
			}
		}
		return true
	})
	return foundQueryGroups
}
