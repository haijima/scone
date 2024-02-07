package analysis

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/haijima/scone/internal/sql"
)

type AnalyzeMode int

type Option struct {
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

	commentedNodes []*NodeWithPackage
}

type NodeWithPackage struct {
	ast.Node
	Package *types.Package
}

func (o *Option) IsCommented(pkg *types.Package, pos ...token.Pos) bool {
	if pkg == nil || pkg.Path() == "" {
		return false
	}
	for _, p := range pos {
		if p.IsValid() {
			for _, n := range o.commentedNodes {
				if n.Package != nil && n.Node != nil && n.Pos() <= p && p < n.End() {
					return true
				}
			}
		}
	}
	return false
}

func (o *Option) Filter(q *sql.Query, meta *Meta) bool {
	pkgName := meta.Package.Pkg.Name()
	pkgPath := meta.Package.Pkg.Path()
	file := meta.Position().Filename
	funcName := meta.Func.Name()
	queryType := q.Kind.String()
	tables := q.Tables
	hash := q.Sha()

	return !o.IsCommented(meta.Package.Pkg, meta.Pos...) &&
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
