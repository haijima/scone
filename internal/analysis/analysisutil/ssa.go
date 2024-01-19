package analysisutil

import (
	"go/constant"
	"go/token"
	"regexp"
	"slices"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

func GetPosition(pkg *ssa.Package, pos []token.Pos) token.Position {
	i := slices.IndexFunc(pos, func(p token.Pos) bool { return p.IsValid() })
	if i == -1 {
		return pkg.Prog.Fset.Position(token.NoPos)
	}
	return pkg.Prog.Fset.Position(pos[i])
}

var fmtVerbRegexp = regexp.MustCompile(`(^|[^%]|(?:%%)+)(%(?:-?\d+|\+|#)?)(\w)`)

func ConstLikeStringValues(v ssa.Value) ([]string, bool) {
	switch t := v.(type) {
	case *ssa.Const:
		if t.Value != nil && t.Value.Kind() == constant.String {
			if s, err := Unquote(t.Value.ExactString()); err == nil {
				return []string{s}, true
			}
		}
	case *ssa.BinOp:
		x, xok := ConstLikeStringValues(t.X)
		y, yok := ConstLikeStringValues(t.Y)
		if xok && yok && t.Op == token.ADD {
			res := make([]string, 0, len(x)*len(y))
			for _, xx := range x {
				for _, yy := range y {
					res = append(res, xx+yy)
				}
			}
			return res, len(res) > 0
		}
	case *ssa.Phi:
		res := make([]string, 0, len(t.Edges))
		for _, edge := range t.Edges {
			if c, ok := ConstLikeStringValues(edge); ok {
				res = append(res, c...)
			}
		}
		return res, len(res) > 0
	case *ssa.Call:
		common := t.Common()
		if cvFn, ok := common.Value.(*ssa.Function); ok {
			// fmt.Sprintf
			if cvFn.Pkg != nil && cvFn.Pkg.Pkg.Path() == "fmt" && cvFn.Name() == "Sprintf" {
				if fs, ok := ConstLikeStringValues(t.Call.Args[0]); ok && len(fs) == 1 {
					f := fmtVerbRegexp.ReplaceAllStringFunc(fs[0], func(s string) string {
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
						default:
							return s
						}
					})
					if !fmtVerbRegexp.MatchString(f) {
						return []string{f}, true
					}
				}
			}
		}
	}
	return []string{}, false
}

func Unquote(str string) (string, error) {
	if len(str) >= 2 {
		if str[0] == '"' && str[len(str)-1] == '"' {
			return strconv.Unquote(str)
		}
		if str[0] == '\'' && str[len(str)-1] == '\'' {
			return strconv.Unquote(str)
		}
		if str[0] == '`' && str[len(str)-1] == '`' {
			return strconv.Unquote(str)
		}
	}
	return str, nil
}
