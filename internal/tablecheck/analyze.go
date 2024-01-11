package tablecheck

import (
	"github.com/haijima/scone/internal/tablecheck/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

type QueryResultWithSSA struct {
	QueryResult *query.Result
	SSA         *buildssa.SSA
}

func Analyze(dir, pattern string, opt *query.QueryOption) ([]*QueryResultWithSSA, error) {
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
