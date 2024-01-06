package tablecheck

import (
	"github.com/haijima/scone/internal/tablecheck/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

func Analyze(dir, pattern string) (*buildssa.SSA, *query.Result, error) {
	pkgs, err := LoadPackages(dir, pattern)
	if err != nil {
		return nil, nil, err
	}

	ssa, err := BuildSSA(pkgs[0])
	if err != nil {
		return nil, nil, err
	}

	queryResult, err := query.ExtractQuery(ssa)
	if err != nil {
		return nil, nil, err
	}

	return ssa, queryResult, nil
}
