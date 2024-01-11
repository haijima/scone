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
	Doc:  "tablecheck is ...",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		return ExtractQuery(ssaProg, pass.Files, &QueryOption{})
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(new(Result)),
}

type Result struct {
	Queries []*Query
}

type QueryOption struct {
	ExcludeQueries      []string
	ExcludePackages     []string
	ExcludePackagePaths []string
	ExcludeFiles        []string
	ExcludeFunctions    []string
	ExcludeQueryTypes   []string
	ExcludeTables       []string
	FilterQueries       []string
	FilterPackages      []string
	FilterPackagePaths  []string
	FilterFiles         []string
	FilterFunctions     []string
	FilterQueryTypes    []string
	FilterTables        []string

	queryCommentPositions []token.Pos
}

func ExtractQuery(ssaProg *buildssa.SSA, files []*ast.File, opt *QueryOption) (*Result, error) {
	foundQueries := make([]*Query, 0)
	opt.queryCommentPositions = make([]token.Pos, 0)

	// Get queries from comments
	foundQueries = append(foundQueries, getQueriesInComment(ssaProg, files, opt)...)

	for _, member := range ssaProg.SrcFuncs {
		foundQueries = append(foundQueries, analyzeFunc(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		//foundQueries = append(foundQueries, analyzeFuncByAst(member, opt)...)
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

func filter(q *Query, opt *QueryOption) bool {
	pkgName := q.Package.Pkg.Name()
	pkgPath := q.Package.Pkg.Path()
	file := q.Position().Filename
	funcName := q.Func.Name()
	queryType := q.Kind.String()
	table := q.Tables[0]
	hash := q.Sha()

	return filterAndExclude(pkgName, opt.FilterPackages, opt.ExcludePackages) &&
		filterAndExclude(pkgPath, opt.FilterPackagePaths, opt.ExcludePackagePaths) &&
		filterAndExclude(file, opt.FilterFiles, opt.ExcludeFiles) &&
		filterAndExclude(funcName, opt.FilterFunctions, opt.ExcludeFunctions) &&
		filterAndExclude(queryType, opt.FilterQueryTypes, opt.ExcludeQueryTypes) &&
		filterAndExclude(table, opt.FilterTables, opt.ExcludeTables) &&
		filterAndExcludeFunc(hash, opt.FilterQueries, opt.ExcludeQueries, strings.HasPrefix)
}

func filterAndExclude(target string, filters []string, excludes []string) bool {
	return filterAndExcludeFunc(target, filters, excludes, func(target, input string) bool { return target == input })
}

func filterAndExcludeFunc(target string, filters []string, excludes []string, fn func(target, input string) bool) bool {
	match := true
	if filters != nil && len(filters) > 0 {
		match = false
		for _, f := range filters {
			if fn(target, f) {
				match = true
			}
		}
	}
	if excludes != nil && len(excludes) > 0 {
		for _, e := range excludes {
			if fn(target, e) {
				match = false
			}
		}
	}
	return match
}
