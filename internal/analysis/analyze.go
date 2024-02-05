package analysis

import (
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/haijima/scone/internal/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

type QueryResultWithSSA struct {
	QueryGroups []*query.QueryGroup
	SSA         *buildssa.SSA
}

func Analyze(dir, pattern string, opt *Option) ([]*query.QueryGroup, mapset.Set[string], []*CallGraph, error) {
	result, err := analyzeSSA(dir, pattern, opt)
	if err != nil {
		return nil, nil, nil, err
	}
	tables := mapset.NewSet[string]()
	queryGroups := make([]*query.QueryGroup, 0)
	cgs := make([]*CallGraph, 0, len(result))
	for _, res := range result {
		for _, qg := range res.QueryGroups {
			queryGroups = append(queryGroups, qg)
			for _, q := range qg.List {
				for _, t := range q.Tables {
					tables.Add(t)
				}
			}
		}
		cg, err := BuildCallGraph(res.SSA, res.QueryGroups)
		if err != nil {
			return nil, nil, nil, err
		}
		cgs = append(cgs, cg)
	}
	return queryGroups, tables, cgs, nil
}

func analyzeSSA(dir, pattern string, opt *Option) ([]*QueryResultWithSSA, error) {
	pkgs, err := analysisutil.LoadPackages(dir, pattern)
	if err != nil {
		return nil, err
	}

	results := make([]*QueryResultWithSSA, 0, len(pkgs))
	for _, pkg := range pkgs {
		ssa, err := analysisutil.BuildSSA(pkg)
		if err != nil {
			return nil, err
		}

		queryGroups, err := ExtractQuery(ssa, pkg.Syntax, opt)
		if err != nil {
			return nil, err
		}

		results = append(results, &QueryResultWithSSA{QueryGroups: queryGroups, SSA: ssa})
	}

	return results, nil
}
