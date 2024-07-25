package analysis

import (
	"context"
	"go/ast"
	"log/slog"
	"slices"

	"github.com/haijima/analysisutil/astutil"
	"github.com/haijima/analysisutil/ssautil"
	"github.com/haijima/scone/internal/sql"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// ExtractQuery extracts queries from the given package.
func ExtractQuery(ctx context.Context, ssaProg *buildssa.SSA, files []*ast.File, opt *Option) (QueryResults, error) {
	// Get queries from comments
	foundQueryResults := handleComments(ctx, ssaProg, files, opt)

	// Get queries from source code
	for _, member := range ssaProg.SrcFuncs {
		foundQueryResults = slices.Concat(foundQueryResults, AnalyzeFunc(ctx, member, opt))
	}

	// Sort and compact
	slices.SortFunc(foundQueryResults, func(a, b *QueryResult) int { return a.Compare(b) })
	return slices.CompactFunc(foundQueryResults, func(a, b *QueryResult) bool { return a.Compare(b) == 0 }), nil
}

func handleComments(ctx context.Context, ssaProg *buildssa.SSA, files []*ast.File, opt *Option) []*QueryResult {
	foundQueryResults := make([]*QueryResult, 0)
	astutil.WalkCommentGroup(ssaProg.Pkg.Prog.Fset, files, func(n ast.Node, cg *ast.CommentGroup) bool {
		qr := NewQueryResult(NewMeta(&ssa.Function{}, cg.Pos()))
		qr.Meta.FromComment = true
		if i := slices.IndexFunc(ssaProg.SrcFuncs, func(fn *ssa.Function) bool { return astutil.Include(fn.Syntax(), n) }); i >= 0 {
			qr.Meta.Func = ssaProg.SrcFuncs[i]
			qr.Meta.Pos = append(qr.Meta.Pos, ssaProg.SrcFuncs[i].Pos())
		}
		for _, comment := range cg.List {
			v, arg, _ := astutil.GetCommentVerb(comment, "scone")
			switch v {
			case "sql":
				if q, ok := sql.ParseString(arg); ok && opt.Filter(q, qr.Meta) {
					qr.Append(q)
					opt.commentedNodes = append(opt.commentedNodes, &NodeWithPackage{Node: n, Package: ssaProg.Pkg.Pkg})
				} else {
					slog.WarnContext(ctx, "Failed to parse string as SQL in scone:sql comment", slog.Any("string", arg), slog.Any("analysis", qr.Meta))
				}
			case "ignore":
				opt.commentedNodes = append(opt.commentedNodes, &NodeWithPackage{Node: n, Package: ssaProg.Pkg.Pkg})
			}
		}
		foundQueryResults = append(foundQueryResults, qr)
		return true
	})
	return slices.DeleteFunc(foundQueryResults, func(qr *QueryResult) bool { return len(qr.Queries()) == 0 })
}

func AnalyzeFunc(ctx context.Context, fn *ssa.Function, opt *Option) QueryResults {
	foundQueryResults := make([]*QueryResult, 0)
	// Analyze anonymous functions recursively
	for _, anon := range fn.AnonFuncs {
		foundQueryResults = slices.Concat(foundQueryResults, AnalyzeFunc(ctx, anon, opt))
	}

	seen := map[*ssa.CallCommon]bool{}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			// 1. Check if the instruction is a call
			callCommon, ok := ssautil.InstrToCallCommon(instr)
			if !ok || seen[callCommon] {
				continue
			}
			seen[callCommon] = true

			// 2. Check if the call is a target function and extract the target argument
			targetArg, ok := CheckIfTargetFunction(ctx, callCommon, opt)
			if !ok {
				if slog.Default().Enabled(ctx, slog.LevelWarn) {
					c := ssautil.GetCallInfo(callCommon)
					for i := 0; i < c.ArgsLen(); i++ {
						if strs, ok := ssautil.ValueToStrings(c.Arg(i)); ok {
							for _, str := range strs {
								if q, ok := sql.ParseString(str); ok {
									meta := NewMeta(fn, c.Arg(i).Pos(), callCommon.Pos(), instr.Pos(), fn.Pos())
									slog.WarnContext(ctx, "Found a query in a non-target function", slog.Any("call", callCommon), slog.Any("SQL", q), slog.Any("analysis", meta))
								}
							}
						}
					}
				}
				continue
			}

			// 3. ssa.Value to filtered sql.Query
			meta := NewMeta(fn, targetArg.Pos(), callCommon.Pos(), instr.Pos(), fn.Pos())
			qr := valueToValidQuery(ctx, targetArg, opt, meta)
			if qr != nil && len(qr.Queries()) > 0 {
				foundQueryResults = append(foundQueryResults, qr)
			}
		}
	}

	return foundQueryResults
}

func valueToValidQuery(ctx context.Context, v ssa.Value, opt *Option, meta *Meta) *QueryResult {

	// 3-1. ssa.Value to string constants.
	// Returns a slice considering the case where the argument value is a Phi node.
	strs, ok := ssautil.ValueToStrings(v)
	if !ok {
		if reason := unknownQueryIfNotSkipped(v, opt, meta); reason != "" {
			slog.InfoContext(ctx, "Failed to convert ssa.Value to string constants: but warning is suppressed", slog.String("reason", string(reason)), slog.Any("value", v), slog.Any("analysis", meta))
			return nil
		}
		slog.WarnContext(ctx, "Failed to convert ssa.Value to string constants", slog.Any("value", v), slog.Any("analysis", meta))
		return &QueryResult{QueryGroup: sql.NewQueryGroupFrom(&sql.Query{Kind: sql.Unknown}), Meta: meta}
	}

	qr := NewQueryResult(meta)
	slices.Sort(strs)
	strs = slices.Compact(strs)
	for _, str := range strs {
		// 3-2. Convert string constants to sql.Query
		q, ok := sql.ParseString(str)
		if !ok {
			if reason := unknownQueryIfNotSkipped(v, opt, meta); reason != "" {
				slog.InfoContext(ctx, "Failed to parse string as SQL: but warning is suppressed", slog.String("reason", string(reason)), slog.Any("string", str), slog.Any("analysis", meta))
			} else {
				slog.WarnContext(ctx, "Failed to parse string as SQL", slog.Any("string", str), slog.Any("analysis", meta))
				qr.Append(&sql.Query{Kind: sql.Unknown})
			}
			continue
		}

		// 3-3. Filter query
		if !opt.Filter(q, meta) {
			slog.InfoContext(ctx, "Filtered query out", slog.Any("SQL", q), slog.Any("analysis", meta))
			continue
		}
		qr.Append(q)
	}
	return qr
}

type TargetCall struct {
	NamePattern string // function name pattern
	ArgIndex    int
}

// Use knife to cut the target function.
// knife -template knife.template database/sql | sort | uniq
// knife -template knife.template github.com/jmoiron/sqlx | sort | uniq
var targetCalls = []TargetCall{
	{NamePattern: "(*database/sql.*).Exec", ArgIndex: 0},
	{NamePattern: "(*database/sql.*).ExecContext", ArgIndex: 1},
	{NamePattern: "(*database/sql.*).Prepare", ArgIndex: 0},
	{NamePattern: "(*database/sql.*).PrepareContext", ArgIndex: 1},
	{NamePattern: "(*database/sql.*).Query", ArgIndex: 0},
	{NamePattern: "(*database/sql.*).QueryContext", ArgIndex: 1},
	{NamePattern: "(*database/sql.*).QueryRow", ArgIndex: 0},
	{NamePattern: "(*database/sql.*).QueryRowContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).BindNamed", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Exec", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).ExecContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Get", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).GetContext", ArgIndex: 2},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).MustExec", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).MustExecContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).NamedExec", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).NamedExecContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).NamedQuery", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).NamedQueryContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Prepare", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).PrepareContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).PrepareNamed", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).PrepareNamedContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Preparex", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).PreparexContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Query", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).QueryContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).QueryRow", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).QueryRowContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).QueryRowx", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).QueryRowxContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Queryx", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).QueryxContext", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Rebind", ArgIndex: 0},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).Select", ArgIndex: 1},
	{NamePattern: "(*github.com/jmoiron/sqlx.*).SelectContext", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.*.BindNamed", ArgIndex: 0}, // not sure
	{NamePattern: "github.com/jmoiron/sqlx.*.Exec", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.*.ExecContext", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.*.Prepare", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.*.PrepareContext", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.*.Query", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.*.QueryContext", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.*.QueryRowx", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.*.QueryRowxContext", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.*.Queryx", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.*.QueryxContext", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.*.Rebind", ArgIndex: 0}, // not sure
	{NamePattern: "github.com/jmoiron/sqlx.BindNamed", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.Get", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.GetContext", ArgIndex: 3},
	{NamePattern: "github.com/jmoiron/sqlx.In", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.MustExec", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.MustExecContext", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.Named", ArgIndex: 0},
	{NamePattern: "github.com/jmoiron/sqlx.NamedExec", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.NamedExecContext", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.NamedQuery", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.NamedQueryContext", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.Preparex", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.PreparexContext", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.Rebind", ArgIndex: 1},
	{NamePattern: "github.com/jmoiron/sqlx.Select", ArgIndex: 2},
	{NamePattern: "github.com/jmoiron/sqlx.SelectContext", ArgIndex: 3},
}

func CheckIfTargetFunction(_ context.Context, call *ssa.CallCommon, opt *Option) (ssa.Value, bool) {
	c := ssautil.GetCallInfo(call)
	for _, t := range slices.Concat(targetCalls, opt.AdditionalFuncSlice()) {
		if c.Match(t.NamePattern) {
			return c.Arg(t.ArgIndex), true
		}
	}
	return nil, false
}

type skipReason string

func unknownQueryIfNotSkipped(v ssa.Value, opt *Option, meta *Meta) skipReason {
	if opt.IsCommented(meta.Package(), meta.Pos...) {
		return "No need to warn if v is commented by scone:sql or scone:ignore"
	} else if !opt.Filter(&sql.Query{Kind: sql.Unknown}, meta) {
		return "No need to warn if v is filtered out"
	}
	if call, ok := ssautil.ValueToCallCommon(v); ok {
		switch ssautil.GetCallInfo(call).Name() {
		case "(*github.com/jmoiron/sqlx.*).BindNamed", "github.com/jmoiron/sqlx.*.BindNamed", "github.com/jmoiron/sqlx.BindNamed":
			return "No need to warn if v is the result of BindNamed()"
		case "(*github.com/jmoiron/sqlx.*).Rebind", "github.com/jmoiron/sqlx.*.Rebind", "github.com/jmoiron/sqlx.Rebind":
			return "No need to warn if v is the result of Rebind()"
		case "github.com/jmoiron/sqlx.In":
			return "No need to warn if v is the result of In()"
		}
	}
	return ""
}
