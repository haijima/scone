package query

import (
	"go/ast"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

func analyzeFuncByAst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
	foundQueries := make([]*Query, 0)
	ast.Inspect(fn.Syntax(), func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if q, ok := toSqlQuery(lit.Value); ok {
				q.Func = fn
				q.Pos = append([]token.Pos{lit.Pos()}, pos...)
				q.Package = pkg
				if filter(q, opt) {
					foundQueries = append(foundQueries, q)
				}
			}
		}
		return true
	})
	return foundQueries
}
