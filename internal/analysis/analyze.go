package analysis

import (
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"golang.org/x/tools/go/ssa"
)

func Analyze(dir, pattern string, opt *Option) ([]*QueryResult, mapset.Set[string], []*CallGraph, error) {
	results, err := analyzeSSA(dir, pattern, opt)
	if err != nil {
		return nil, nil, nil, err
	}
	tables := mapset.NewSet[string]()
	qrsByPkg := make(map[*ssa.Package][]*QueryResult)
	for _, qr := range results {
		for _, q := range qr.Queries() {
			for _, t := range q.Tables {
				tables.Add(t)
			}
		}
		qrsByPkg[qr.Meta.Package] = append(qrsByPkg[qr.Meta.Package], qr)
	}
	cgs := make([]*CallGraph, 0, len(results))
	for pkg, qrs := range qrsByPkg {
		cg, err := BuildCallGraph(pkg, qrs)
		if err != nil {
			return nil, nil, nil, err
		}
		cgs = append(cgs, cg)
	}
	return results, tables, cgs, nil
}

func analyzeSSA(dir, pattern string, opt *Option) ([]*QueryResult, error) {
	pkgs, err := analysisutil.LoadPackages(dir, pattern)
	if err != nil {
		return nil, err
	}

	results := make([]*QueryResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		ssa, err := analysisutil.BuildSSA(pkg)
		if err != nil {
			return nil, err
		}

		queryResults, err := ExtractQuery(ssa, pkg.Syntax, opt)
		if err != nil {
			return nil, err
		}

		results = append(results, queryResults...)
	}

	return results, nil
}
