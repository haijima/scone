package analysis

import (
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"slices"
	"strconv"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/haijima/scone/internal/sql"
	"golang.org/x/exp/maps"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

type QueryResults []*QueryResult

func (qrs QueryResults) AllTables() []*sql.Table {
	s := maps.Values(qrs.allTableMap())
	slices.SortFunc(s, func(a, b *sql.Table) int { return strings.Compare(a.Name, b.Name) })
	return s
}

func (qrs QueryResults) AllTableNames() []string {
	return mapset.Sorted(mapset.NewSetFromMapKeys(qrs.allTableMap()))
}

func (qrs QueryResults) allTableMap() map[string]*sql.Table {
	qgs := make([]*sql.QueryGroup, 0)
	for _, qr := range qrs {
		qgs = append(qgs, qr.QueryGroup)
	}
	return sql.QueryGroups(qgs).AllTableMap()
}

type QueryResult struct {
	*sql.QueryGroup
	Meta *Meta
}

func (qr *QueryResult) Append(qs ...*sql.Query) {
	if qr.QueryGroup == nil {
		qr.QueryGroup = sql.NewQueryGroup()
	} else if qr.List == nil {
		qr.List = mapset.NewSet[*sql.Query]()
	}
	qr.List.Append(qs...)
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

func (m *Meta) Compare(other *Meta) int {
	if m.Package.Pkg.Path() != other.Package.Pkg.Path() {
		return strings.Compare(m.Package.Pkg.Path(), other.Package.Pkg.Path())
	} else if m.Position().Filename != other.Position().Filename {
		return strings.Compare(m.Position().Filename, other.Position().Filename)
	} else if m.Position().Offset != other.Position().Offset {
		return m.Position().Offset - other.Position().Offset
	}
	return 0
}

func (m *Meta) Equal(other *Meta) bool {
	return m.Compare(other) == 0
}

// ExtractQuery extracts queries from the given package.
func ExtractQuery(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) (QueryResults, error) {
	foundQueryResults := make([]*QueryResult, 0)

	// Get queries from comments
	for _, file := range files {
		cm := ast.NewCommentMap(ssaProg.Pkg.Prog.Fset, file, file.Comments)
		for n, cgs := range cm {
			for _, cg := range cgs {
				qr := &QueryResult{Meta: NewMeta(ssaProg.Pkg, &ssa.Function{}, cg.Pos())}
				for _, fn := range ssaProg.SrcFuncs {
					if fn.Syntax().Pos() <= n.Pos() && n.End() <= fn.Syntax().End() {
						qr.Meta.Func = fn
						qr.Meta.Pos = append(qr.Meta.Pos, fn.Pos())
						break
					}
				}
				for _, comment := range cg.List {
					if strings.HasPrefix(comment.Text, "// scone:sql") {
						if q, ok := sql.ParseString(strings.TrimPrefix(comment.Text, "// scone:sql")); ok && opt.Filter(q, qr.Meta) {
							qr.Append(q)
						}
					}
					if strings.HasPrefix(comment.Text, "// scone:sql") || strings.HasPrefix(comment.Text, "// scone:ignore") {
						opt.commentedNodes = append(opt.commentedNodes, &NodeWithPackage{Node: n, Package: ssaProg.Pkg.Pkg})
					}
				}
				if len(qr.Queries()) > 0 {
					foundQueryResults = append(foundQueryResults, qr)
				}
			}
		}
	}

	for _, member := range ssaProg.SrcFuncs {
		foundQueryResults = append(foundQueryResults, AnalyzeFuncBySsaMethod(ssaProg.Pkg, member, []token.Pos{}, opt)...)
	}

	slices.SortFunc(foundQueryResults, func(a, b *QueryResult) int {
		if !a.Meta.Equal(b.Meta) {
			return a.Meta.Compare(b.Meta)
		}
		return slices.CompareFunc(a.Queries(), b.Queries(), func(a, b *sql.Query) int { return strings.Compare(a.Sha(), b.Sha()) })
	})
	foundQueryResults = slices.CompactFunc(foundQueryResults, func(a, b *QueryResult) bool {
		return a.Meta.Equal(b.Meta) && slices.EqualFunc(a.Queries(), b.Queries(), func(a, b *sql.Query) bool { return a.Sha() == b.Sha() })
	})
	return foundQueryResults, nil
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

func AnalyzeFuncBySsaMethod(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) QueryResults {
	tms := make([]methodArg, len(targetMethods))
	copy(tms, targetMethods)
	if opt.AdditionalFuncs != nil || len(opt.AdditionalFuncs) > 0 {
		for _, f := range opt.AdditionalFuncs {
			s := strings.Split(f, "#")
			if len(s) != 3 {
				slog.Warn(fmt.Sprintf("Invalid format of additional function: %s", f))
				continue
			}
			if idx, err := strconv.Atoi(s[2]); err == nil {
				tms = append(tms, methodArg{Package: s[0], Method: s[1], ArgIndex: idx})
				continue
			}
			slog.Warn(fmt.Sprintf("Index of additional function should be integer: %s", f))
		}
	}

	foundQueryResults := make([]*QueryResult, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			if c, ok := toCallCommon(instr); ok {
				for _, t := range tms {
					if analysisutil.IsFunc(c, t.Package, t.Method) {
						idx := t.ArgIndex
						if !c.IsInvoke() {
							idx++ // Set first argument as receiver
						}
						arg := c.Args[idx]
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

	for _, anon := range fn.AnonFuncs {
		foundQueryResults = append(foundQueryResults, AnalyzeFuncBySsaMethod(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}

	return foundQueryResults
}

func toCallCommon(instr ssa.Instruction) (*ssa.CallCommon, bool) {
	switch i := instr.(type) {
	case *ssa.Call:
		return i.Common(), true
	case *ssa.Extract:
		if call, ok := i.Tuple.(*ssa.Call); ok {
			return call.Common(), true
		}
	case *ssa.Go:
		return i.Common(), true
	case *ssa.Defer:
		return i.Common(), true
	}
	return nil, false
}

func constLikeStringValueToQueryGroup(pkg *ssa.Package, v ssa.Value, fn *ssa.Function, pos []token.Pos, opt *Option) (*QueryResult, bool) {
	meta := NewMeta(pkg, fn, v.Pos(), pos...)
	if as, ok := analysisutil.ConstLikeStringValues(v); ok {
		qr := &QueryResult{Meta: meta}
		for _, a := range as {
			if q, ok := sql.ParseString(a); ok {
				if opt.Filter(q, meta) {
					qr.Append(q)
				} else {
					slog.Debug("filtered out: sql", "SQL", q.String(), "package", pkg.Pkg.Path(), "file", analysisutil.FLC(meta.Position()))
				}
			} else {
				if norm, err := sql.Normalize(a); err == nil {
					a = norm
				}
				if uq := unknownQueryIfNotSkipped(v, opt, meta, "Can't parse string as SQL", "SQL", a); uq != nil {
					qr.Append(uq)
				}
			}
		}
		if len(qr.Queries()) > 0 {
			return qr, true
		}
		return &QueryResult{}, false
	} else {
		if uq := unknownQueryIfNotSkipped(v, opt, meta, "Can't parse value as string constant", "value", fmt.Sprintf("%v", v)); uq != nil {
			return &QueryResult{QueryGroup: sql.NewQueryGroupFrom(uq), Meta: NewMeta(pkg, fn, v.Pos(), pos...)}, true
		}
		return &QueryResult{}, false
	}
}

func unknownQueryIfNotSkipped(v ssa.Value, opt *Option, meta *Meta, logMessage string, logArgs ...any) *sql.Query {
	logArgs = append(logArgs, "package", meta.Package.Pkg.Path(), "file", analysisutil.FLC(meta.Position()), "function", meta.Func.Name())
	if reason, skipped := skip(v, opt, meta); skipped {
		slog.Debug(fmt.Sprintf("%s: but warning is suppressed", logMessage), append([]any{"reason", reason}, logArgs...)...)
		return nil
	}
	slog.Warn(logMessage, logArgs...)
	return &sql.Query{Kind: sql.Unknown}
}

// skip returns the reason why the given value is skipped. If the value is skipped, the second return value should be true.
// if skipped, warning should be suppressed and unknown query is not added to the result.
func skip(v ssa.Value, opt *Option, meta *Meta) (string, bool) {
	if opt.IsCommented(meta.Package.Pkg, meta.Pos...) {
		return "No need to warn if v is commented by scone:sql or scone:ignore", true
	} else if !opt.Filter(&sql.Query{Kind: sql.Unknown}, meta) {
		return "No need to warn if v is filtered out", true
	}
	switch t := v.(type) {
	case *ssa.Call:
		if analysisutil.IsFunc(t.Common(), "github.com/jmoiron/sqlx", "Rebind") {
			return "No need to warn if v is the result of sqlx.Rebind()", true
		}
	case *ssa.Extract:
		if c, ok := t.Tuple.(*ssa.Call); ok && analysisutil.IsFunc(c.Common(), "github.com/jmoiron/sqlx", "In") {
			return "No need to warn if v is the result of sqlx.In()", true
		}
	}
	return "", false
}
