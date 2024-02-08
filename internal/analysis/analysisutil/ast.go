package analysisutil

import (
	"go/ast"
	"go/token"
)

func Include(u, v ast.Node) bool {
	return u.Pos() <= v.Pos() && v.End() <= u.End()
}

func WalkCommentGroup(fset *token.FileSet, files []*ast.File, fn func(node ast.Node, cg *ast.CommentGroup) bool) {
	for _, file := range files {
		for n, cgs := range ast.NewCommentMap(fset, file, file.Comments) {
			for _, cg := range cgs {
				if !fn(n, cg) {
					break
				}
			}
		}
	}
}
