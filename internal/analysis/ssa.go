package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/haijima/scone/internal/query"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func GetQueryGroupsInComment(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) []*query.QueryGroup {
	foundQueryGroups := make([]*query.QueryGroup, 0)

	commentPrefix := "// scone:sql"
	for _, file := range files {
		for _, cg := range file.Comments {
			for _, comment := range cg.List {
				qg := &query.QueryGroup{}
				if strings.HasPrefix(comment.Text, commentPrefix) {
					if q, ok := query.ToSqlQuery(strings.TrimPrefix(comment.Text, commentPrefix)); ok {
						q.Func = &ssa.Function{}
						for _, member := range ssaProg.SrcFuncs {
							if member.Syntax().Pos() <= comment.Pos() && comment.End() <= member.Syntax().End() {
								q.Func = member
								break
							}
						}
						q.Pos = append([]token.Pos{comment.Pos()})
						if q.Func != nil {
							q.Pos = append(q.Pos, q.Func.Pos())
						}
						q.Package = ssaProg.Pkg
						if opt.Filter(q) {
							qg.List = append(qg.List, q)
							opt.QueryCommentPositions = append(opt.QueryCommentPositions, comment.Pos())
						}
					}
				}
				if len(qg.List) > 0 {
					foundQueryGroups = append(foundQueryGroups, qg)
				}
			}
		}
	}
	return foundQueryGroups
}

func AnalyzeFuncBySsaConst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*query.QueryGroup {
	foundQueryGroups := make([]*query.QueryGroup, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			foundQueryGroups = append(foundQueryGroups, instructionToQueryGroups(pkg, instr, append([]token.Pos{fn.Pos()}, pos...), opt)...)
		}
	}
	for _, anon := range fn.AnonFuncs {
		foundQueryGroups = append(foundQueryGroups, AnalyzeFuncBySsaConst(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}
	return foundQueryGroups
}

func instructionToQueryGroups(pkg *ssa.Package, instr ssa.Instruction, pos []token.Pos, opt *Option) []*query.QueryGroup {
	switch i := instr.(type) {
	case *ssa.Call:
		return callToQueryGroups(pkg, i, instr.Parent(), pos, opt)
	case *ssa.Phi:
		return []*query.QueryGroup{phiToQueryGroup(pkg, i, instr.Parent(), pos, opt)}
	}
	return []*query.QueryGroup{}
}

func callToQueryGroups(pkg *ssa.Package, i *ssa.Call, fn *ssa.Function, pos []token.Pos, opt *Option) []*query.QueryGroup {
	res := make([]*query.QueryGroup, 0)
	pos = append([]token.Pos{i.Pos()}, pos...)
	for _, arg := range i.Common().Args {
		switch a := arg.(type) {
		case *ssa.Phi:
			res = append(res, phiToQueryGroup(pkg, a, fn, pos, opt))
		case *ssa.Const:
			if q, ok := constToQueryGroup(pkg, a, fn, pos, opt); ok {
				res = append(res, q)
			}
		}
	}
	return res
}

func phiToQueryGroup(pkg *ssa.Package, a *ssa.Phi, fn *ssa.Function, pos []token.Pos, opt *Option) *query.QueryGroup {
	qg := &query.QueryGroup{}
	for _, edge := range a.Edges {
		switch e := edge.(type) {
		case *ssa.Const:
			if qg, ok := constToQueryGroup(pkg, e, fn, append([]token.Pos{a.Pos()}, pos...), opt); ok {
				qg.List = append(qg.List, qg.List...)
			}
		}
	}
	return qg
}

func constToQueryGroup(pkg *ssa.Package, a *ssa.Const, fn *ssa.Function, pos []token.Pos, opt *Option) (*query.QueryGroup, bool) {
	if a.Value != nil && a.Value.Kind() == constant.String {
		if q, ok := query.ToSqlQuery(a.Value.ExactString()); ok {
			q.Func = fn
			q.Pos = append([]token.Pos{a.Pos()}, pos...)
			q.Package = pkg
			if opt.Filter(q) {
				return &query.QueryGroup{List: []*query.Query{q}}, true
			}
		}
	}
	return nil, false
}

type methodArg struct {
	Package  string
	Method   string
	ArgIndex int
}

var targetMethods = []methodArg{
	{Package: "database/sql", Method: "ExecContext", ArgIndex: 1},
	{Package: "database/sql", Method: "Exec", ArgIndex: 0},
	{Package: "database/sql", Method: "QueryContext", ArgIndex: 1},
	{Package: "database/sql", Method: "Query", ArgIndex: 0},
	{Package: "database/sql", Method: "QueryRowContext", ArgIndex: 1},
	{Package: "database/sql", Method: "QueryRow", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "Exec", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "Rebind", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "BindNamed", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "NamedQuery", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "NamedExec", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "Select", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "Get", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "Queryx", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "QueryRowx", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "MustExec", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "Preparex", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "PrepareNamed", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "PreparexContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "PrepareNamedContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "MustExecContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "QueryxContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "SelectContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "GetContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "QueryRowxContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "NamedExecContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "ExecContext", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "In", ArgIndex: -1},
}

func AnalyzeFuncBySsaMethod(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*query.QueryGroup {
	tms := make([]methodArg, len(targetMethods))
	copy(tms, targetMethods)
	if opt.AdditionalFuncs != nil || len(opt.AdditionalFuncs) > 0 {
		for _, f := range opt.AdditionalFuncs {
			s := strings.Split(f, "#")
			if len(s) != 3 {
				slog.Warn(fmt.Sprintf("Invalid format of additional function: %s", f))
				continue
			}
			idx, err := strconv.Atoi(s[2])
			if err != nil {
				slog.Warn(fmt.Sprintf("Index of additional function should be integer: %s", f))
				continue
			}
			tms = append(tms, methodArg{Package: s[0], Method: s[1], ArgIndex: idx})
		}
	}

	foundQueryGroups := make([]*query.QueryGroup, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if c, ok := instr.(*ssa.Call); ok {
				common := c.Common()
				if mp, mn, ok := analysisutil.GetFuncInfo(common); ok {
					for _, t := range tms {
						if mp == t.Package && mn == t.Method {
							idx := t.ArgIndex
							if !common.IsInvoke() {
								idx++ // Set first argument as receiver
							}
							arg := common.Args[idx]
							if phi, ok := arg.(*ssa.Phi); ok {
								for _, edge := range phi.Edges {
									if qg, ok := constLikeStringValueToQueryGroup(pkg, edge, fn, append([]token.Pos{arg.Pos(), c.Pos(), fn.Pos()}, pos...), opt); ok {
										foundQueryGroups = append(foundQueryGroups, qg)
									}
								}
							} else if qg, ok := constLikeStringValueToQueryGroup(pkg, arg, fn, append([]token.Pos{c.Pos(), fn.Pos()}, pos...), opt); ok {
								foundQueryGroups = append(foundQueryGroups, qg)
							}
							break // Found target method
						}
					}
				}
			}
		}
	}

	for _, anon := range fn.AnonFuncs {
		foundQueryGroups = append(foundQueryGroups, AnalyzeFuncBySsaMethod(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}

	return foundQueryGroups
}

func constLikeStringValueToQueryGroup(pkg *ssa.Package, v ssa.Value, fn *ssa.Function, pos []token.Pos, opt *Option) (*query.QueryGroup, bool) {
	file := analysisutil.FLC(analysisutil.GetPosition(pkg, append([]token.Pos{v.Pos()}, pos...)))
	if as, ok := analysisutil.ConstLikeStringValues(v); ok {
		qg := &query.QueryGroup{}
		for _, a := range as {
			if q, ok := query.ToSqlQuery(a); ok {
				q.Func = fn
				q.Pos = append([]token.Pos{v.Pos()}, pos...)
				q.Package = pkg
				if opt.Filter(q) {
					qg.List = append(qg.List, q)
				} else {
					slog.Debug("filtered", "SQL", v, "package", pkg.Pkg.Path(), "file", file)
					return &query.QueryGroup{}, false
				}
			} else {
				level := slog.LevelWarn
				if IsCommented(pkg, append([]token.Pos{v.Pos()}, pos...), opt) {
					level = slog.LevelDebug
				}
				if norm, err := query.Normalize(a); err == nil {
					a = norm
				}
				slog.Log(context.Background(), level, "Cannot parse as SQL", "SQL", a, "package", pkg.Pkg.Path(), "file", file, "function", fn.Name())
			}
		}
		if len(qg.List) > 0 {
			slices.SortFunc(qg.List, func(a, b *query.Query) int { return strings.Compare(a.Raw, b.Raw) })
			qg.List = slices.CompactFunc(qg.List, func(a, b *query.Query) bool { return a.Raw == b.Raw })
			return qg, true
		}
	} else {
		if extract, ok := v.(*ssa.Extract); ok {
			if c, ok := extract.Tuple.(*ssa.Call); ok {
				if fn, ok := c.Common().Value.(*ssa.Function); ok {
					if fn.Pkg != nil && fn.Pkg.Pkg.Path() == "github.com/jmoiron/sqlx" && fn.Name() == "In" {
						// No need to warn if v is the result of sqlx.In()
						return &query.QueryGroup{}, false
					}
				}
			}
		}
		if IsCommented(pkg, append([]token.Pos{v.Pos()}, pos...), opt) {
			slog.Debug("Can't parse value as string constant", "type", fmt.Sprintf("%T", v), "value", fmt.Sprintf("%v", v), "package", pkg.Pkg.Path(), "file", file, "function", fn.Name())
			return &query.QueryGroup{}, false
		}
		slog.Warn("Can't parse value as string constant", "type", fmt.Sprintf("%T", v), "value", fmt.Sprintf("%v", v), "package", pkg.Pkg.Path(), "file", file, "function", fn.Name())
	}
	q := &query.Query{Kind: query.Unknown, Func: fn, Pos: append([]token.Pos{v.Pos()}, pos...), Package: pkg}
	if opt.Filter(q) {
		return &query.QueryGroup{List: []*query.Query{q}}, true
	}
	return &query.QueryGroup{}, false
}

func IsCommented(pkg *ssa.Package, pos []token.Pos, opt *Option) bool {
	position := analysisutil.GetPosition(pkg, pos)
	commented := false
	for _, cp := range opt.QueryCommentPositions {
		if analysisutil.GetPosition(pkg, append([]token.Pos{cp})).Line == position.Line-1 {
			commented = true
		}
	}

	for _, p := range pos {
		if p.IsValid() {
			commented = commented || opt.IsIgnoredFunc(p)
		}
	}
	return commented
}
