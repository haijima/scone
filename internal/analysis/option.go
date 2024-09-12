package analysis

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log/slog"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/haijima/scone/internal/sql"
)

type AnalyzeMode int

type Option struct {
	Code            string
	AdditionalFuncs []string
	expr            *FilterExpr
	commentedNodes  []*NodeWithPackage
}

type NodeWithPackage struct {
	ast.Node
	Package *types.Package
}

func NewOption(code string, additionalFuncs []string) *Option {
	opt := &Option{
		Code:            code,
		AdditionalFuncs: additionalFuncs,
	}
	expr, err := NewFilterExpr(code)
	if err != nil {
		panic(err)
	}
	opt.expr = expr
	return opt
}

func (o *Option) IsCommented(pkg *types.Package, pos ...token.Pos) bool {
	if pkg == nil || pkg.Path() == "" {
		return false
	}
	for _, p := range pos {
		if p.IsValid() {
			for _, n := range o.commentedNodes {
				if n.Package != nil && n.Node != nil && n.Pos() <= p && p < n.End() {
					return true
				}
			}
		}
	}
	return false
}

func (o *Option) AdditionalFuncSlice() []TargetCall {
	tms := make([]TargetCall, 0)
	if o.AdditionalFuncs != nil || len(o.AdditionalFuncs) > 0 {
		for _, f := range o.AdditionalFuncs {
			s := strings.Split(f, "@")
			if len(s) != 2 {
				slog.Warn(fmt.Sprintf("Invalid format of additional function: %s", f))
			} else if idx, err := strconv.Atoi(s[1]); err == nil {
				tms = append(tms, TargetCall{NamePattern: s[0], ArgIndex: idx})
			} else {
				slog.Warn(fmt.Sprintf("Index of additional function should be integer: %s", f))
			}
		}
	}
	return tms
}

func (o *Option) Filter(q *sql.Query, meta *Meta) bool {
	if o.expr == nil {
		return true
	}
	res, err := o.expr.Run(q, meta)
	if err != nil {
		panic(err)
	}
	return res
}

type FilterExpr struct {
	program cel.Program
}

func NewFilterExpr(code string) (*FilterExpr, error) {
	fmt.Println("code: ", code)

	if code == "" {
		code = "true"
	}
	env, err := cel.NewEnv(
		cel.Variable("pkgName", cel.StringType),
		cel.Variable("pkgPath", cel.StringType),
		cel.Variable("file", cel.StringType),
		cel.Variable("func", cel.StringType),
		cel.Variable("queryType", cel.StringType),
		cel.Variable("tables", cel.ListType(cel.StringType)),
		cel.Variable("hash", cel.StringType),
	)
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(code)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}
	prg, err := env.Program(ast)
	if err != nil {
		return nil, err
	}

	return &FilterExpr{program: prg}, nil
}

func (f *FilterExpr) Run(q *sql.Query, meta *Meta) (bool, error) {
	pkgName := meta.Package().Name()
	pkgPath := meta.Package().Path()
	file := meta.Position().Filename
	funcName := meta.Func.Name()
	queryType := q.Kind.String()
	tables := q.Tables
	hash := q.Hash()

	out, _, err := f.program.Eval(map[string]any{
		"pkgName":   pkgName,
		"pkgPath":   pkgPath,
		"file":      file,
		"func":      funcName,
		"queryType": queryType,
		"tables":    tables,
		"hash":      hash,
	})
	if err != nil {
		return false, err
	}

	b, ok := out.Value().(bool)
	if !ok {
		return false, errors.New("not a bool")
	}
	return b, nil
}
