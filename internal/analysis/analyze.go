package analysis

import (
	"slices"

	"github.com/haijima/scone/internal/analysis/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

type QueryResultWithSSA struct {
	QueryResult *query.Result
	SSA         *buildssa.SSA
}

func Analyze(dir, pattern string, opt *query.Option) ([]*QueryResultWithSSA, error) {
	pkgs, err := LoadPackages(dir, pattern)
	if err != nil {
		return nil, err
	}

	results := make([]*QueryResultWithSSA, 0, len(pkgs))
	for _, pkg := range pkgs {
		ssa, err := BuildSSA(pkg)
		if err != nil {
			return nil, err
		}

		queryResult, err := query.ExtractQuery(ssa, pkg.Syntax, opt)
		if err != nil {
			return nil, err
		}

		results = append(results, &QueryResultWithSSA{QueryResult: queryResult, SSA: ssa})
	}

	return results, nil
}

func GetQueriesAndTablesFromResult(result []*QueryResultWithSSA) ([]*query.Query, []string) {
	tables := make([]string, 0)
	queries := make([]*query.Query, 0)
	for _, res := range result {
		for _, q := range res.QueryResult.Queries {
			queries = append(queries, q)
			for _, t := range q.Tables {
				tables = append(tables, t)
			}
		}
	}
	slices.Sort(tables)
	tables = slices.Compact(tables)

	return queries, tables
}
