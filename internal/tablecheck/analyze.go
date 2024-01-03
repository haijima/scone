package tablecheck

import "golang.org/x/tools/go/analysis/passes/buildssa"

func Analyze(dir, pattern string) (*buildssa.SSA, *QueryResult, error) {
	pkgs, err := LoadPackages(dir, pattern)
	if err != nil {
		return nil, nil, err
	}

	ssa, err := BuildSSA(pkgs[0])
	if err != nil {
		return nil, nil, err
	}

	queryResult, err := ExtractQuery(ssa)
	if err != nil {
		return nil, nil, err
	}

	return ssa, queryResult, nil
}
