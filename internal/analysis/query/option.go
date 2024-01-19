package query

import (
	"go/token"
	"slices"
	"strings"
)

type AnalyzeMode int

const (
	SsaMethod AnalyzeMode = iota
	SsaConst
	Ast
)

type Option struct {
	Mode                AnalyzeMode
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
	AdditionalFuncs     []string

	queryCommentPositions []token.Pos
	isIgnoredFunc         func(pos token.Pos) bool
}

func (o *Option) Filter(q *Query) bool {
	pkgName := q.Package.Pkg.Name()
	pkgPath := q.Package.Pkg.Path()
	file := q.Position().Filename
	funcName := q.Func.Name()
	queryType := q.Kind.String()
	tables := q.Tables
	hash := q.Sha()

	commented := false
	for _, p := range q.Pos {
		if p.IsValid() {
			commented = commented || o.isIgnoredFunc(p)
		}
	}

	return !commented &&
		(slices.Contains(o.FilterPackages, pkgName) || len(o.FilterPackages) == 0) &&
		(!slices.Contains(o.ExcludePackages, pkgName) || len(o.ExcludePackages) == 0) &&
		(slices.Contains(o.FilterPackagePaths, pkgPath) || len(o.FilterPackagePaths) == 0) &&
		(!slices.Contains(o.ExcludePackagePaths, pkgPath) || len(o.ExcludePackagePaths) == 0) &&
		(slices.Contains(o.FilterFiles, file) || len(o.FilterFiles) == 0) &&
		(!slices.Contains(o.ExcludeFiles, file) || len(o.ExcludeFiles) == 0) &&
		(slices.Contains(o.FilterFunctions, funcName) || len(o.FilterFunctions) == 0) &&
		(!slices.Contains(o.ExcludeFunctions, funcName) || len(o.ExcludeFunctions) == 0) &&
		(slices.Contains(o.FilterQueryTypes, queryType) || len(o.FilterQueryTypes) == 0) &&
		(!slices.Contains(o.ExcludeQueryTypes, queryType) || len(o.ExcludeQueryTypes) == 0) &&
		(slices.ContainsFunc(o.FilterTables, func(s string) bool { return slices.Contains(tables, s) }) || len(o.FilterTables) == 0) &&
		(!slices.ContainsFunc(o.ExcludeTables, func(s string) bool { return slices.Contains(tables, s) }) || len(o.ExcludeTables) == 0) &&
		(slices.ContainsFunc(o.FilterQueries, func(s string) bool { return strings.HasPrefix(hash, s) }) || len(o.FilterQueries) == 0) &&
		(!slices.ContainsFunc(o.ExcludeQueries, func(s string) bool { return strings.HasPrefix(hash, s) }) || len(o.ExcludeQueries) == 0)
}
