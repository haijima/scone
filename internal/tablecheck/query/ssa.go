package query

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func getQueriesInComment(ssaProg *buildssa.SSA, files []*ast.File, opt *Option) []*Query {
	foundQueries := make([]*Query, 0)

	commentPrefix := "// tablecheck:sql"
	for _, file := range files {
		for _, cg := range file.Comments {
			for _, comment := range cg.List {
				if strings.HasPrefix(comment.Text, commentPrefix) {
					if q, ok := toSqlQuery(strings.TrimPrefix(comment.Text, commentPrefix)); ok {
						q.Func = &ssa.Function{}
						for _, member := range ssaProg.SrcFuncs {
							if member.Syntax().Pos() <= comment.Pos() && comment.End() <= member.Syntax().End() {
								q.Func = member
								break
							}
						}
						q.Pos = append([]token.Pos{comment.Pos()})
						q.Package = ssaProg.Pkg
						if filter(q, opt) {
							foundQueries = append(foundQueries, q)
							opt.queryCommentPositions = append(opt.queryCommentPositions, comment.Pos())
						}
					}
				}
			}
		}
	}
	return foundQueries
}

func analyzeFuncBySsaConst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*Query {
	foundQueries := make([]*Query, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			foundQueries = append(foundQueries, analyzeInstr(pkg, instr, append([]token.Pos{fn.Pos()}, pos...), opt)...)
		}
	}
	for _, anon := range fn.AnonFuncs {
		foundQueries = append(foundQueries, analyzeFuncBySsaConst(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}
	return foundQueries
}

func analyzeInstr(pkg *ssa.Package, instr ssa.Instruction, pos []token.Pos, opt *Option) []*Query {
	foundQueries := make([]*Query, 0)
	switch i := instr.(type) {
	case *ssa.Call:
		foundQueries = append(foundQueries, callToQueries(pkg, i, instr.Parent(), pos, opt)...)
	case *ssa.Phi:
		foundQueries = append(foundQueries, phiToQueries(pkg, i, instr.Parent(), pos, opt)...)
	}
	return foundQueries
}

func callToQueries(pkg *ssa.Package, i *ssa.Call, fn *ssa.Function, pos []token.Pos, opt *Option) []*Query {
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

func phiToQueries(pkg *ssa.Package, a *ssa.Phi, fn *ssa.Function, pos []token.Pos, opt *Option) []*Query {
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

func constToQuery(pkg *ssa.Package, a *ssa.Const, fn *ssa.Function, pos []token.Pos, opt *Option) (*Query, bool) {
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

type methodArg struct {
	Package  string
	Method   string
	ArgIndex int
}

var targetMethods = []methodArg{
	{Package: "database/sql", Method: "ExecContext", ArgIndex: 2},
	{Package: "database/sql", Method: "Exec", ArgIndex: 1},
	{Package: "database/sql", Method: "QueryContext", ArgIndex: 2},
	{Package: "database/sql", Method: "Query", ArgIndex: 1},
	{Package: "database/sql", Method: "QueryRowContext", ArgIndex: 2},
	{Package: "database/sql", Method: "QueryRow", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "Exec", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "Rebind", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "BindNamed", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "NamedQuery", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "NamedExec", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "Select", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "Get", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "Queryx", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "QueryRowx", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "MustExec", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "Preparex", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "PrepareNamed", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "PreparexContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "PrepareNamedContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "MustExecContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "QueryxContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "SelectContext", ArgIndex: 3},
	{Package: "github.com/jmoiron/sqlx", Method: "GetContext", ArgIndex: 3},
	{Package: "github.com/jmoiron/sqlx", Method: "QueryRowxContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "NamedExecContext", ArgIndex: 2},
	{Package: "github.com/jmoiron/sqlx", Method: "Exec", ArgIndex: 1},
	{Package: "github.com/jmoiron/sqlx", Method: "In", ArgIndex: 0},
}

func analyzeFuncBySsaMethod(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*Query {
	foundQueries := make([]*Query, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			c, ok1 := instr.(*ssa.Call)
			if !ok1 {
				continue
			}
			m, ok2 := c.Call.Value.(*ssa.Function)
			if !ok2 {
				continue
			}
			for _, t := range targetMethods {
				if m.Pkg != nil && m.Pkg.Pkg.Path() == t.Package && m.Name() == t.Method {
					arg := c.Common().Args[t.ArgIndex]
					if a, ok := arg.(*ssa.Const); ok {
						if q, ok := constToQuery(pkg, a, fn, append([]token.Pos{c.Pos(), fn.Pos()}, pos...), opt); ok {
							foundQueries = append(foundQueries, q)
						}
					} else if p, ok := arg.(*ssa.Phi); ok {
						for _, edge := range p.Edges {
							if e, ok := edge.(*ssa.Const); ok {
								if q, ok := constToQuery(pkg, e, fn, append([]token.Pos{c.Pos(), fn.Pos()}, pos...), opt); ok {
									foundQueries = append(foundQueries, q)
								}
							} else {
								warnIfNotCommented(pkg, edge, append([]token.Pos{e.Pos(), p.Pos(), c.Pos(), fn.Pos()}, pos...), opt)
							}
						}
					} else {
						warnIfNotCommented(pkg, arg, append([]token.Pos{c.Pos(), fn.Pos()}, pos...), opt)
					}
					break
				}
			}
		}
	}

	for _, anon := range fn.AnonFuncs {
		foundQueries = append(foundQueries, analyzeFuncBySsaMethod(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}

	return foundQueries
}

func IsCommented(pkg *ssa.Package, pos []token.Pos, opt *Option) bool {
	position := GetPosition(pkg, pos)
	commented := false
	for _, cp := range opt.queryCommentPositions {
		if GetPosition(pkg, append([]token.Pos{cp})).Line == position.Line-1 {
			commented = true
		}
	}

	for _, p := range pos {
		if p.IsValid() {
			commented = commented || opt.isIgnoredFunc(p)
		}
	}
	return commented
}

func warnIfNotCommented(pkg *ssa.Package, v ssa.Value, pos []token.Pos, opt *Option) {
	position := GetPosition(pkg, pos)
	if !IsCommented(pkg, pos, opt) {
		file := fmt.Sprintf("%s:%d:%d", filepath.Base(position.Filename), position.Line, position.Column)
		slog.Warn("Cannot parse query", "SQL", v, "package", pkg.Pkg.Path(), "file", file)
	}
}
