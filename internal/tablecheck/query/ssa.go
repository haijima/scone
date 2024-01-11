package query

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

func analyzeFunc(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
	foundQueries := make([]*Query, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			foundQueries = append(foundQueries, analyzeInstr(pkg, instr, append([]token.Pos{fn.Pos()}, pos...), opt)...)
		}
	}
	for _, anon := range fn.AnonFuncs {
		foundQueries = append(foundQueries, analyzeFunc(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}
	return foundQueries
}

func analyzeInstr(pkg *ssa.Package, instr ssa.Instruction, pos []token.Pos, opt *QueryOption) []*Query {
	foundQueries := make([]*Query, 0)
	switch i := instr.(type) {
	case *ssa.Call:
		foundQueries = append(foundQueries, callToQueries(pkg, i, instr.Parent(), pos, opt)...)
	case *ssa.Phi:
		foundQueries = append(foundQueries, phiToQueries(pkg, i, instr.Parent(), pos, opt)...)
	}
	return foundQueries
}

func callToQueries(pkg *ssa.Package, i *ssa.Call, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
	res := make([]*Query, 0)
	pos = append([]token.Pos{i.Pos()}, pos...)
	for _, arg := range i.Common().Args {
		switch a := arg.(type) {
		case *ssa.Phi:
			res = append(res, phiToQueries(pkg, a, fn, pos, opt)...)
		case *ssa.Const:
			if q, ok := constToQuery(pkg, a, fn, pos, opt); ok {
				res = append(res, q)
			}
		}
	}
	return res
}

func phiToQueries(pkg *ssa.Package, a *ssa.Phi, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
	res := make([]*Query, 0)
	for _, edge := range a.Edges {
		switch e := edge.(type) {
		case *ssa.Const:
			if q, ok := constToQuery(pkg, e, fn, append([]token.Pos{a.Pos()}, pos...), opt); ok {
				res = append(res, q)
			}
		}
	}
	return res
}

func constToQuery(pkg *ssa.Package, a *ssa.Const, fn *ssa.Function, pos []token.Pos, opt *QueryOption) (*Query, bool) {
	if a.Value != nil && a.Value.Kind() == constant.String {
		if q, ok := toSqlQuery(a.Value.ExactString()); ok {
			q.Func = fn
			q.Pos = append([]token.Pos{a.Pos()}, pos...)
			q.Package = pkg
			if filter(q, opt) {
				return q, true
			}
		}
	}
	return nil, false
}
