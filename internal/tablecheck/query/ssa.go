package query

import (
	"context"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"
	"path/filepath"
	"regexp"
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
						if q.Func != nil {
							q.Pos = append(q.Pos, q.Func.Pos())
						}
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
	{Package: "github.com/jmoiron/sqlx", Method: "Exec", ArgIndex: 0},
	{Package: "github.com/jmoiron/sqlx", Method: "In", ArgIndex: -1},

	{Package: "github.com/isucon/isucon12-qualify/webapp/go", Method: "GetContext", ArgIndex: 2},
	{Package: "github.com/isucon/isucon12-qualify/webapp/go", Method: "SelectContext", ArgIndex: 2},
	{Package: "github.com/isucon/isucon12-qualify/webapp/go", Method: "ExecContext", ArgIndex: 1},
}

func analyzeFuncBySsaMethod(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *Option) []*Query {
	foundQueries := make([]*Query, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			c, ok := instr.(*ssa.Call)
			if !ok {
				continue
			}
			common := c.Common()

			var mp, mn string
			if common.IsInvoke() && common.Method.Pkg() != nil {
				mp = common.Method.Pkg().Path()
				mn = common.Method.Name()
			} else if m, ok := common.Value.(*ssa.Function); ok {
				if m.Pkg != nil {
					mp = m.Pkg.Pkg.Path()
				} else if m.Signature.Recv() != nil && m.Signature.Recv().Pkg() != nil {
					mp = m.Signature.Recv().Pkg().Path()
				}
				mn = m.Name()
			} else {
				continue // Can't get package name of the function
			}

			for _, t := range targetMethods {
				if mp == t.Package && mn == t.Method {
					idx := t.ArgIndex
					if !common.IsInvoke() {
						idx++ // Set first argument as receiver
					}
					arg := common.Args[idx]

					if phi, ok := arg.(*ssa.Phi); ok {
						for _, edge := range phi.Edges {
							if q, ok := constLikeStringValueToQuery(pkg, edge, fn, append([]token.Pos{arg.Pos(), c.Pos(), fn.Pos()}, pos...), opt); ok {
								foundQueries = append(foundQueries, q)
							}
						}
					} else if q, ok := constLikeStringValueToQuery(pkg, arg, fn, append([]token.Pos{c.Pos(), fn.Pos()}, pos...), opt); ok {
						foundQueries = append(foundQueries, q)
					}
					break // Found target method
				}
			}
		}
	}

	for _, anon := range fn.AnonFuncs {
		foundQueries = append(foundQueries, analyzeFuncBySsaMethod(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}

	return foundQueries
}

func constLikeStringValueToQuery(pkg *ssa.Package, v ssa.Value, fn *ssa.Function, pos []token.Pos, opt *Option) (*Query, bool) {
	position := GetPosition(pkg, append([]token.Pos{v.Pos()}, pos...))
	file := fmt.Sprintf("%s:%d:%d", filepath.Base(position.Filename), position.Line, position.Column)
	if a, ok := constLikeStringValue(v); ok {
		if q, ok := toSqlQuery(a); ok {
			q.Func = fn
			q.Pos = append([]token.Pos{v.Pos()}, pos...)
			q.Package = pkg
			if filter(q, opt) {
				return q, true
			} else {
				slog.Debug("filtered", "SQL", v, "package", pkg.Pkg.Path(), "file", file)
			}
		} else {
			level := slog.LevelWarn
			if IsCommented(pkg, append([]token.Pos{v.Pos()}, pos...), opt) {
				level = slog.LevelDebug
			}
			if norm, err := normalize(
				a); err == nil {
				slog.Log(context.Background(), level, "Cannot parse query", "SQL", norm, "package", pkg.Pkg.Path(), "file", file)
			} else {
				slog.Log(context.Background(), level, "Cannot parse query", "SQL", a, "package", pkg.Pkg.Path(), "file", file)
			}
		}
	} else {
		if extract, ok := v.(*ssa.Extract); ok {
			if c, ok := extract.Tuple.(*ssa.Call); ok {
				if fn, ok := c.Common().Value.(*ssa.Function); ok {
					if fn.Pkg != nil && fn.Pkg.Pkg.Path() == "github.com/jmoiron/sqlx" && fn.Name() == "In" {
						// No need to warn if v is the result of sqlx.In()
						return nil, false
					}
				}
			}
		}
		level := slog.LevelWarn
		if IsCommented(pkg, append([]token.Pos{v.Pos()}, pos...), opt) {
			level = slog.LevelDebug
		}
		slog.Log(context.Background(), level,
			"Can't parse value as string constant", "value", fmt.Sprintf("%T %v", v, v), "package", pkg.Pkg.Path(), "file", file)
	}
	return nil, false
}

var fmtVerbRegexp = regexp.MustCompile(`(^|[^%]|(?:%%)+)(%(?:-?\d+|\+|#)?)(\w)`)

func constLikeStringValue(v ssa.Value) (string, bool) {
	switch t := v.(type) {
	case *ssa.Const:
		if t.Value != nil && t.Value.Kind() == constant.String {
			return t.Value.ExactString(), true
		}
	case *ssa.BinOp:
		if t.Op == token.ADD {
			if x, ok := constLikeStringValue(t.X); ok {
				if y, ok := constLikeStringValue(t.Y); ok {
					if xx, err := unquote(x); err == nil {
						if yy, err := unquote(y); err == nil {
							return xx + " " + yy, true
						}
					}
				}
			}
		}
	// TODO:Support fmt.Sprintf() and strings.Join()
	case *ssa.Call:
		common := t.Common()
		if cvFn, ok := common.Value.(*ssa.Function); ok {
			if cvFn.Pkg != nil && cvFn.Pkg.Pkg.Path() == "fmt" && cvFn.Name() == "Sprintf" {
				f, ok := constLikeStringValue(t.Call.Args[0])
				if !ok {
					break
				}
				f, err := unquote(f)
				if err != nil {
					break
				}

				i := 0
				f = fmtVerbRegexp.ReplaceAllStringFunc(f, func(s string) string {
					i = i + 1

					m := fmtVerbRegexp.FindAllStringSubmatch(s, 1)
					if m == nil || len(m) < 1 || len(m[0]) < 4 {
						return s
					}
					switch m[0][3] {
					case "b":
						return m[0][1] + "01"
					case "c":
						return m[0][1] + "a"
					case "t":
						return m[0][1] + "true"
					case "T":
						return m[0][1] + "string"
					case "e":
						return m[0][1] + "1.234000e+08"
					case "E":
						return m[0][1] + "1.234000E+08"
					case "p":
						return m[0][1] + "0xc0000ba000"
					case "x":
						return m[0][1] + "1f"
					case "d":
						return m[0][1] + "1"
					case "f":
						return m[0][1] + "1.0"
					//case "s":
					//	if i+1 >= len(t.Call.Args) {
					//		return s
					//	}
					//	a := t.Call.Args[i+1]
					//	if c, ok := constLikeStringValue(a); ok {
					//		return m[0][1] + c
					//	} else {
					//		return s
					//	}
					default:
						return s
					}
				})
				if !fmtVerbRegexp.MatchString(f) {
					return f, true
				}
			}
		}
	}
	return "", false
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
