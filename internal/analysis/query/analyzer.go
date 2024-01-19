package query

import (
	"go/ast"
	"go/token"
	"reflect"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

// Analyzer is ...
var Analyzer = &analysis.Analyzer{
	Name: "extractquery",
	Doc:  "scone is ...",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		return ExtractQuery(ssaProg, pass.Files, &Option{})
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(new(Result)),
}

type Result struct {
	Queries []*Query
}

// ExtractQuery extracts queries from the given package.
func ExtractQuery(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) (*Result, error) {
	foundQueries := make([]*Query, 0)
	opt.queryCommentPositions = make([]token.Pos, 0)
	opt.isIgnoredFunc = func(pos token.Pos) bool { return false }

	// Get queries from comments
	foundQueries = append(foundQueries, getQueriesInComment(ssaProg, files, opt)...)

	//ignoreCommentPrefix := "// scone:ignore"
	for _, file := range files {
		cm := ast.NewCommentMap(ssaProg.Pkg.Prog.Fset, file, file.Comments)
		for n, cgs := range cm {
			for _, cg := range cgs {
				for _, c := range strings.Split(cg.Text(), "\n") {
					if strings.HasPrefix(c, "scone:ignore") {
						old := opt.isIgnoredFunc
						start := n.Pos()
						end := n.End()
						opt.isIgnoredFunc = func(pos token.Pos) bool {
							return old(pos) || (start <= pos && pos < end)
						}
						break
					}
				}
			}
		}
	}

	for _, member := range ssaProg.SrcFuncs {
		switch opt.Mode {
		case SsaMethod:
			foundQueries = append(foundQueries, analyzeFuncBySsaMethod(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		case SsaConst:
			foundQueries = append(foundQueries, analyzeFuncBySsaConst(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		case Ast:
			foundQueries = append(foundQueries, analyzeFuncByAst(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		}
	}

	slices.SortFunc(foundQueries, func(a, b *Query) int {
		if a.Position().Offset != b.Position().Offset {
			return a.Position().Offset - b.Position().Offset
		}
		return strings.Compare(a.Raw, b.Raw)
	})
	foundQueries = slices.CompactFunc(foundQueries, func(a, b *Query) bool {
		return a.Raw == b.Raw && a.Position().Offset == b.Position().Offset
	})
	return &Result{Queries: foundQueries}, nil
}
