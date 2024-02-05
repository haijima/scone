package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/haijima/scone/internal/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/callgraph/static"
	"golang.org/x/tools/go/ssa"
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

func BuildCallGraph(ssaProg *buildssa.SSA, qgs []*query.QueryGroup) (*CallGraph, error) {
	result := &CallGraph{
		Package: ssaProg.Pkg.Pkg,
		Nodes:   make(map[string]*Node),
	}
	cg := static.CallGraph(ssaProg.Pkg.Prog)
	callerFuncs := make([]*ssa.Function, 0, len(qgs))
	queryEdgeMemo := make(map[string]bool)
	for _, qg := range qgs {
		for _, q := range qg.List {
			for _, t := range q.Tables {
				k := fmt.Sprintf("%s#%s#%s", q.Func.Name(), q.Kind, t)
				if queryEdgeMemo[k] {
					continue
				}
				queryEdgeMemo[k] = true

				if q.Func.Name() == "main" || q.Func.Name() == "init" {
					continue
				}
				result.AddQueryEdge(q.Func, t, &SqlValue{Kind: q.Kind, RawSQL: q.Raw})

				callerFuncs = append(callerFuncs, q.Func)
			}
		}
	}

	seen := make(map[*ssa.Function]bool)
	for len(callerFuncs) > 0 {
		fn := callerFuncs[0]
		callerFuncs = callerFuncs[1:]
		if seen[fn] {
			continue
		}
		seen[fn] = true
		if node, ok := cg.Nodes[fn]; ok {
			for _, edge := range node.In {
				caller := edge.Caller.Func
				result.AddFuncCallEdge(caller, fn)

				callerFuncs = append(callerFuncs, caller)
			}
		}
	}
	return result, nil
}

// ExtractQuery extracts queries from the given package.
func ExtractQuery(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) ([]*query.QueryGroup, error) {
	foundQueryGroups := make([]*query.QueryGroup, 0)
	opt.QueryCommentPositions = make([]token.Pos, 0)
	opt.IsIgnoredFunc = func(pos token.Pos) bool { return false }

	// Get queries from comments
	foundQueryGroups = append(foundQueryGroups, GetQueryGroupsInComment(ssaProg, files, opt)...)

	//ignoreCommentPrefix := "// scone:ignore"
	for _, file := range files {
		cm := ast.NewCommentMap(ssaProg.Pkg.Prog.Fset, file, file.Comments)
		for n, cgs := range cm {
			for _, cg := range cgs {
				for _, c := range strings.Split(cg.Text(), "\n") {
					if strings.HasPrefix(c, "scone:ignore") {
						old := opt.IsIgnoredFunc
						start := n.Pos()
						end := n.End()
						opt.IsIgnoredFunc = func(pos token.Pos) bool {
							return old(pos) || (start <= pos && pos < end)
						}
						break
					}
				}
			}
		}
	}

	for _, member := range ssaProg.SrcFuncs {
		switch opt.Mode {
		case SsaMethod:
			foundQueryGroups = append(foundQueryGroups, AnalyzeFuncBySsaMethod(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		case SsaConst:
			foundQueryGroups = append(foundQueryGroups, AnalyzeFuncBySsaConst(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		case Ast:
			foundQueryGroups = append(foundQueryGroups, AnalyzeFuncByAst(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		}
	}

	slices.SortFunc(foundQueryGroups, func(a, b *query.QueryGroup) int {
		if a.List[0].Position().Offset != b.List[0].Position().Offset {
			return a.List[0].Position().Offset - b.List[0].Position().Offset
		}
		return strings.Compare(a.List[0].Raw, b.List[0].Raw)
	})
	foundQueryGroups = slices.CompactFunc(foundQueryGroups, func(a, b *query.QueryGroup) bool {
		return a.List[0].Raw == b.List[0].Raw && a.List[0].Position().Offset == b.List[0].Position().Offset
	})
	return foundQueryGroups, nil
}
