package analysis

import (
	"go/ast"
	"go/token"

	"github.com/haijima/scone/internal/query"
	"golang.org/x/tools/go/ssa"
)

func AnalyzeFuncByAst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*query.QueryGroup {
	foundQueryGroups := make([]*query.QueryGroup, 0)
	ast.Inspect(fn.Syntax(), func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if q, ok := query.ToSqlQuery(lit.Value); ok {
				q.Func = fn
				q.Pos = append([]token.Pos{lit.Pos()}, pos...)
				q.Package = pkg
				if opt.Filter(q) {
					foundQueryGroups = append(foundQueryGroups, &query.QueryGroup{List: []*query.Query{q}})
				}
			}
		}
		return true
	})
	return foundQueryGroups
}
