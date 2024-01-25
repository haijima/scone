package analysis

import (
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/callgraph"
	"github.com/haijima/scone/internal/analysis/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

type QueryResultWithSSA struct {
	QueryResult *query.Result
	SSA         *buildssa.SSA
}

func Analyze(dir, pattern string, opt *query.Option) ([]*query.Query, mapset.Set[string], []*callgraph.CallGraph, error) {
	result, err := analyzeSSA(dir, pattern, opt)
	if err != nil {
		return nil, nil, nil, err
	}
	tables := mapset.NewSet[string]()
	queries := make([]*query.Query, 0)
	cgs := make([]*callgraph.CallGraph, 0, len(result))
	for _, res := range result {
		for _, q := range res.QueryResult.Queries {
			queries = append(queries, q)
			for _, t := range q.Tables {
				tables.Add(t)
			}
		}
		cg, err := callgraph.BuildCallGraph(res.SSA, res.QueryResult)
		if err != nil {
			return nil, nil, nil, err
		}
		cgs = append(cgs, cg)
	}
	return queries, tables, cgs, nil
}

func analyzeSSA(dir, pattern string, opt *query.Option) ([]*QueryResultWithSSA, error) {
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
