package query

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

func analyzeFuncByAst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*QueryGroup {
	foundQueryGroups := make([]*QueryGroup, 0)
	ast.Inspect(fn.Syntax(), func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if q, ok := toSqlQuery(lit.Value); ok {
				q.Func = fn
				q.Pos = append([]token.Pos{lit.Pos()}, pos...)
				q.Package = pkg
				if opt.Filter(q) {
					foundQueryGroups = append(foundQueryGroups, &QueryGroup{List: []*Query{q}})
				}
			}
		}
		return true
	})
	return foundQueryGroups
}
