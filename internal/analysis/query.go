package analysis

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"log/slog"
	"slices"
	"strings"

	"github.com/haijima/scone/internal/analysis/analysisutil"
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
		foundQueryResults = append(foundQueryResults, AnalyzeFunc(ctx, member, []token.Pos{}, opt)...)
	}

	// Sort and compact
	slices.SortFunc(foundQueryResults, func(a, b *QueryResult) int { return a.Compare(b) })
	return slices.CompactFunc(foundQueryResults, func(a, b *QueryResult) bool { return a.Compare(b) == 0 }), nil
}

func handleComments(ctx context.Context, ssaProg *buildssa.SSA, files []*ast.File, opt *Option) []*QueryResult {
	foundQueryResults := make([]*QueryResult, 0)
	analysisutil.WalkCommentGroup(ssaProg.Pkg.Prog.Fset, files, func(n ast.Node, cg *ast.CommentGroup) bool {
		qr := NewQueryResult(NewMeta(&ssa.Function{}, cg.Pos()))
		if i := slices.IndexFunc(ssaProg.SrcFuncs, func(fn *ssa.Function) bool { return analysisutil.Include(fn.Syntax(), n) }); i >= 0 {
			qr.Meta.Func = ssaProg.SrcFuncs[i]
			qr.Meta.Pos = append(qr.Meta.Pos, ssaProg.SrcFuncs[i].Pos())
		}
		for _, comment := range cg.List {
			if strings.HasPrefix(comment.Text, "// scone:sql") {
				if q, ok := sql.ParseString(strings.TrimPrefix(comment.Text, "// scone:sql")); ok && opt.Filter(q, qr.Meta) {
					qr.Append(q)
					opt.commentedNodes = append(opt.commentedNodes, &NodeWithPackage{Node: n, Package: ssaProg.Pkg.Pkg})
				}
			} else if strings.HasPrefix(comment.Text, "// scone:ignore") {
				opt.commentedNodes = append(opt.commentedNodes, &NodeWithPackage{Node: n, Package: ssaProg.Pkg.Pkg})
			}
		}
		foundQueryResults = append(foundQueryResults, qr)
		return true
	})
	return slices.DeleteFunc(foundQueryResults, func(qr *QueryResult) bool { return len(qr.Queries()) == 0 })
}

func AnalyzeFunc(ctx context.Context, fn *ssa.Function, pos []token.Pos, opt *Option) QueryResults {
	foundQueryResults := make([]*QueryResult, 0)
	// Analyze anonymous functions recursively
	for _, anon := range fn.AnonFuncs {
		foundQueryResults = append(foundQueryResults, AnalyzeFunc(ctx, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}

	seen := map[*ssa.CallCommon]bool{}
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			// 1. Check if the instruction is a call
			callCommon, ok := analysisutil.InstrToCallCommon(instr)
			if !ok || seen[callCommon] {
				continue
			}
			seen[callCommon] = true

			// 2. Check if the call is a target function and extract the target argument
			targetArg, ok := CheckIfTargetFunction(ctx, callCommon, opt)
			if !ok {
				continue
			}

			// 3. ssa.Value to filtered sql.Query
			qr := valueToValidQuery(ctx, targetArg, fn, append([]token.Pos{callCommon.Pos(), instr.Pos(), fn.Pos()}, pos...), opt)
			if qr != nil && len(qr.Queries()) > 0 {
				foundQueryResults = append(foundQueryResults, qr)
			}
		}
	}

	return foundQueryResults
}

func valueToValidQuery(ctx context.Context, v ssa.Value, fn *ssa.Function, pos []token.Pos, opt *Option) *QueryResult {
	meta := NewMeta(fn, v.Pos(), pos...)

	// 3-1. ssa.Value to string constants.
	// Returns a slice considering the case where the argument value is a Phi node.
	strs, ok := analysisutil.ValueToStrings(v)
	if !ok {
		if q := unknownQueryIfNotSkipped(ctx, v, opt, meta, "Failed to convert ssa.Value to string constants", "value", v); q != nil {
			return &QueryResult{QueryGroup: sql.NewQueryGroupFrom(q), Meta: meta}
		}
		return nil
	}

	qr := NewQueryResult(meta)
	for _, str := range strs {
		// 3-2. Convert string constants to sql.Query
		q, ok := sql.ParseString(str)
		if !ok {
			if q := unknownQueryIfNotSkipped(ctx, v, opt, meta, "Failed to parse string as SQL", "string", str); q != nil {
				qr.Append(q)
			}
			continue
		}

		// 3-3. Filter query
		if !opt.Filter(q, meta) {
			slog.InfoContext(ctx, "Filtered query out", slog.String("SQL", q.String()), meta.LogAttr())
			continue
		}
		qr.Append(q)
	}
	return qr
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

func CheckIfTargetFunction(_ context.Context, c *ssa.CallCommon, opt *Option) (ssa.Value, bool) {
	for _, t := range append(targetMethods, opt.AdditionalFuncSlice()...) {
		if analysisutil.IsFunc(c, t.Package, t.Method) {
			if c.IsInvoke() {
				return c.Args[t.ArgIndex], true
			}
			return c.Args[t.ArgIndex+1], true // Set first argument as receiver
		}
	}
	return nil, false
}

func unknownQueryIfNotSkipped(ctx context.Context, v ssa.Value, opt *Option, meta *Meta, logMessage string, logArgs ...any) *sql.Query {
	suppressedLogMessage := fmt.Sprintf("%s: but warning is suppressed", logMessage)
	if opt.IsCommented(meta.Package(), meta.Pos...) {
		slog.InfoContext(ctx, suppressedLogMessage, append([]any{"reason", "No need to warn if v is commented by scone:sql or scone:ignore"}, logArgs...)...)
		return nil
	} else if !opt.Filter(&sql.Query{Kind: sql.Unknown}, meta) {
		slog.InfoContext(ctx, suppressedLogMessage, append([]any{"reason", "No need to warn if v is filtered out"}, logArgs...)...)
		return nil
	}
	if c, ok := analysisutil.ValueToCallCommon(v); ok {
		if analysisutil.IsFunc(c, "github.com/jmoiron/sqlx", "Rebind") {
			slog.InfoContext(ctx, suppressedLogMessage, append([]any{"reason", "No need to warn if v is the result of sqlx.Rebind()"}, logArgs...)...)
			return nil
		} else if analysisutil.IsFunc(c, "github.com/jmoiron/sqlx", "In") {
			slog.InfoContext(ctx, suppressedLogMessage, append([]any{"reason", "No need to warn if v is the result of sqlx.In()"}, logArgs...)...)
			return nil
		}
	}

	slog.WarnContext(ctx, logMessage, logArgs...)
	return &sql.Query{Kind: sql.Unknown}
}
