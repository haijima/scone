package analysisutil

import (
	"go/ast"
	"go/token"
	"strings"
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

func HasCommentPrefix(c *ast.Comment, prefix string) bool {
	return strings.HasPrefix(c.Text, "// "+prefix) || strings.HasPrefix(c.Text, "//"+prefix)
}

func GetCommentVerb(c *ast.Comment, prefix string) (string, string, bool) {
	if !HasCommentPrefix(c, prefix) {
		return "", "", false
	}
	s := strings.TrimPrefix(c.Text, "// "+prefix)
	s = strings.TrimPrefix(s, "//"+prefix)
	if s[0] != ':' {
		return "", "", false
	}
	v, arg, _ := strings.Cut(s[1:], " ")
	return v, arg, v != ""
}
