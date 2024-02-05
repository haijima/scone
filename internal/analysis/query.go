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

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/haijima/scone/internal/sql"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type QueryResult struct {
	QueryGroup *sql.QueryGroup
	Meta       *Meta
}

func (qr *QueryResult) Queries() []*sql.Query {
	if qr.QueryGroup == nil {
		qr.QueryGroup = sql.NewQueryGroup()
	} else if qr.QueryGroup.List == nil {
		qr.QueryGroup.List = mapset.NewSet[*sql.Query]()
	}
	s := qr.QueryGroup.List.ToSlice()
	slices.SortFunc(s, func(a, b *sql.Query) int { return strings.Compare(a.Raw, b.Raw) })
	return s
}

func (qr *QueryResult) Append(qs ...*sql.Query) {
	if qr.QueryGroup == nil {
		qr.QueryGroup = sql.NewQueryGroup()
	} else if qr.QueryGroup.List == nil {
		qr.QueryGroup.List = mapset.NewSet[*sql.Query]()
	}
	qr.QueryGroup.List.Append(qs...)
}

type Meta struct {
	Package *ssa.Package
	Func    *ssa.Function
	Pos     []token.Pos
}

func NewMeta(pkg *ssa.Package, fn *ssa.Function, pos token.Pos, fallbackPos ...token.Pos) *Meta {
	return &Meta{Package: pkg, Func: fn, Pos: append([]token.Pos{pos}, fallbackPos...)}
}

func (m *Meta) Position() token.Position {
	return analysisutil.GetPosition(m.Package, m.Pos)
}

// ExtractQuery extracts queries from the given package.
func ExtractQuery(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) ([]*QueryResult, error) {
	foundQueryResults := make([]*QueryResult, 0)
	opt.QueryCommentPositions = make([]token.Pos, 0)
	opt.IsIgnoredFunc = func(pos token.Pos) bool { return false }

	// Get queries from comments
	foundQueryResults = append(foundQueryResults, GetQueryResultsInComment(ssaProg, files, opt)...)

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
			foundQueryResults = append(foundQueryResults, AnalyzeFuncBySsaMethod(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		case SsaConst:
			foundQueryResults = append(foundQueryResults, AnalyzeFuncBySsaConst(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		case Ast:
			foundQueryResults = append(foundQueryResults, AnalyzeFuncByAst(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		}
	}

	slices.SortFunc(foundQueryResults, func(a, b *QueryResult) int {
		if a.Meta.Position().Offset != b.Meta.Position().Offset {
			return a.Meta.Position().Offset - b.Meta.Position().Offset
		}
		return strings.Compare(a.Queries()[0].Raw, b.Queries()[0].Raw)
	})
	foundQueryResults = slices.CompactFunc(foundQueryResults, func(a, b *QueryResult) bool {
		return a.Queries()[0].Sha() == b.Queries()[0].Sha() && a.Meta.Position().Offset == b.Meta.Position().Offset
	})
	return foundQueryResults, nil
}

func GetQueryResultsInComment(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) []*QueryResult {
	foundQueryResults := make([]*QueryResult, 0)

	commentPrefix := "// scone:sql"
	for _, file := range files {
		for _, cg := range file.Comments {
			qr := &QueryResult{Meta: NewMeta(ssaProg.Pkg, &ssa.Function{}, cg.Pos())}
			for _, member := range ssaProg.SrcFuncs {
				if member.Syntax().Pos() <= cg.Pos() && cg.End() <= member.Syntax().End() {
					qr.Meta.Func = member
					qr.Meta.Pos = append(qr.Meta.Pos, member.Pos())
					break
				}
			}
			for _, comment := range cg.List {
				if strings.HasPrefix(comment.Text, commentPrefix) {
					if q, ok := sql.ParseString(strings.TrimPrefix(comment.Text, commentPrefix)); ok {
						if opt.Filter(q, qr.Meta) {
							qr.Append(q)
							opt.QueryCommentPositions = append(opt.QueryCommentPositions, comment.Pos())
						}
					}
				}
			}
			if len(qr.Queries()) > 0 {
				foundQueryResults = append(foundQueryResults, qr)
			}
		}
	}
	return foundQueryResults
}

func AnalyzeFuncBySsaConst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*QueryResult {
	foundQueryResults := make([]*QueryResult, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			foundQueryResults = append(foundQueryResults, instructionToQueryGroups(pkg, instr, append([]token.Pos{fn.Pos()}, pos...), opt)...)
		}
	}
	for _, anon := range fn.AnonFuncs {
		foundQueryResults = append(foundQueryResults, AnalyzeFuncBySsaConst(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}
	return foundQueryResults
}

func instructionToQueryGroups(pkg *ssa.Package, instr ssa.Instruction, pos []token.Pos, opt *Option) []*QueryResult {
	switch i := instr.(type) {
	case *ssa.Call:
		return callToQueryGroups(pkg, i, instr.Parent(), pos, opt)
	case *ssa.Phi:
		return []*QueryResult{phiToQueryGroup(pkg, i, instr.Parent(), pos, opt)}
	}
	return []*QueryResult{}
}

func callToQueryGroups(pkg *ssa.Package, i *ssa.Call, fn *ssa.Function, pos []token.Pos, opt *Option) []*QueryResult {
	res := make([]*QueryResult, 0)
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

func phiToQueryGroup(pkg *ssa.Package, a *ssa.Phi, fn *ssa.Function, pos []token.Pos, opt *Option) *QueryResult {
	queryResult := &QueryResult{Meta: NewMeta(pkg, fn, a.Pos(), pos...)}
	for _, edge := range a.Edges {
		switch e := edge.(type) {
		case *ssa.Const:
			if qr, ok := constToQueryGroup(pkg, e, fn, append([]token.Pos{a.Pos()}, pos...), opt); ok {
				queryResult.Append(qr.Queries()...)
			}
		}
	}
	return queryResult
}

func constToQueryGroup(pkg *ssa.Package, a *ssa.Const, fn *ssa.Function, pos []token.Pos, opt *Option) (*QueryResult, bool) {
	if a.Value != nil && a.Value.Kind() == constant.String {
		if q, ok := sql.ParseString(a.Value.ExactString()); ok {
			qr := &QueryResult{QueryGroup: sql.NewQueryGroupFrom(q), Meta: NewMeta(pkg, fn, a.Pos(), pos...)}
			if opt.Filter(q, qr.Meta) {
				return qr, true
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

func AnalyzeFuncBySsaMethod(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*QueryResult {
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

	foundQueryResults := make([]*QueryResult, 0)
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
										foundQueryResults = append(foundQueryResults, qg)
									}
								}
							} else if qg, ok := constLikeStringValueToQueryGroup(pkg, arg, fn, append([]token.Pos{c.Pos(), fn.Pos()}, pos...), opt); ok {
								foundQueryResults = append(foundQueryResults, qg)
							}
							break // Found target method
						}
					}
				}
			}
		}
	}

	for _, anon := range fn.AnonFuncs {
		foundQueryResults = append(foundQueryResults, AnalyzeFuncBySsaMethod(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}

	return foundQueryResults
}

func constLikeStringValueToQueryGroup(pkg *ssa.Package, v ssa.Value, fn *ssa.Function, pos []token.Pos, opt *Option) (*QueryResult, bool) {
	file := analysisutil.FLC(analysisutil.GetPosition(pkg, append([]token.Pos{v.Pos()}, pos...)))
	if as, ok := analysisutil.ConstLikeStringValues(v); ok {
		qr := &QueryResult{Meta: NewMeta(pkg, fn, v.Pos(), pos...)}
		for _, a := range as {
			if q, ok := sql.ParseString(a); ok {
				if opt.Filter(q, qr.Meta) {
					qr.Append(q)
				} else {
					slog.Debug("filtered", "SQL", v, "package", pkg.Pkg.Path(), "file", file)
					return &QueryResult{}, false
				}
			} else {
				level := slog.LevelWarn
				if IsCommented(pkg, append([]token.Pos{v.Pos()}, pos...), opt) {
					level = slog.LevelDebug
				}
				if norm, err := sql.Normalize(a); err == nil {
					a = norm
				}
				slog.Log(context.Background(), level, "Cannot parse as SQL", "SQL", a, "package", pkg.Pkg.Path(), "file", file, "function", fn.Name())
			}
		}
		if len(qr.Queries()) > 0 {
			return qr, true
		}
	} else {
		if extract, ok := v.(*ssa.Extract); ok {
			if c, ok := extract.Tuple.(*ssa.Call); ok {
				if fn, ok := c.Common().Value.(*ssa.Function); ok {
					if fn.Pkg != nil && fn.Pkg.Pkg.Path() == "github.com/jmoiron/sqlx" && fn.Name() == "In" {
						// No need to warn if v is the result of sqlx.In()
						return &QueryResult{}, false
					}
				}
			}
		}
		if IsCommented(pkg, append([]token.Pos{v.Pos()}, pos...), opt) {
			slog.Debug("Can't parse value as string constant", "type", fmt.Sprintf("%T", v), "value", fmt.Sprintf("%v", v), "package", pkg.Pkg.Path(), "file", file, "function", fn.Name())
			return &QueryResult{}, false
		}
		slog.Warn("Can't parse value as string constant", "type", fmt.Sprintf("%T", v), "value", fmt.Sprintf("%v", v), "package", pkg.Pkg.Path(), "file", file, "function", fn.Name())
	}
	q := &sql.Query{Kind: sql.Unknown}
	qr := &QueryResult{QueryGroup: sql.NewQueryGroupFrom(q), Meta: NewMeta(pkg, fn, v.Pos(), pos...)}
	if opt.Filter(q, qr.Meta) {
		return qr, true
	}
	return &QueryResult{}, false
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
