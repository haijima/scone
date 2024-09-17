package analysis

import (
	"context"
	"slices"

	"github.com/haijima/analysisutil"
	"github.com/haijima/analysisutil/ssautil"
	"golang.org/x/tools/go/ssa"
)

func Analyze(ctx context.Context, dir, pattern string, opt *Option) (QueryResults, map[string]*CallGraph, error) {
	results, err := analyzeSSA(ctx, dir, pattern, opt)
	if err != nil {
		return nil, nil, err
	}
	qrsByPkg := make(map[*ssa.Package]QueryResults)
	for _, qr := range results {
		qrsByPkg[qr.Posx.Func.Pkg] = append(qrsByPkg[qr.Posx.Func.Pkg], qr)
	}
	cgs := make(map[string]*CallGraph)
	for pkg, qrs := range qrsByPkg {
		cg, err := BuildCallGraph(pkg, qrs)
		if err != nil {
			return nil, nil, err
		}
		cgs[pkg.Pkg.Path()] = cg
	}
	return results, cgs, nil
}

func analyzeSSA(ctx context.Context, dir, pattern string, opt *Option) (QueryResults, error) {
	pkgs, err := analysisutil.LoadPackages(dir, pattern)
	if err != nil {
		return nil, err
	}

	results := make([]*QueryResult, 0, len(pkgs))
	for _, pkg := range pkgs {
		ssaProg, err := ssautil.BuildSSA(pkg)
		if err != nil {
			return nil, err
		}

		queryResults, err := ExtractQuery(ctx, ssaProg, pkg.Syntax, opt)
		if err != nil {
			return nil, err
		}

		results = slices.Concat(results, queryResults)
	}

	return results, nil
}
