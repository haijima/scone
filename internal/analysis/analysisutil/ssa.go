package analysisutil

import (
	"go/constant"
	"go/token"
	"regexp"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

func GetPosition(pkg *ssa.Package, pos []token.Pos) token.Position {
	res := token.NoPos
	for _, p := range pos {
		if p.IsValid() {
			res = p
			break
		}
	}
	return pkg.Prog.Fset.Position(res)
}

var fmtVerbRegexp = regexp.MustCompile(`(^|[^%]|(?:%%)+)(%(?:-?\d+|\+|#)?)(\w)`)

func ConstLikeStringValues(v ssa.Value) ([]string, bool) {
	switch t := v.(type) {
	case *ssa.Const:
		if t.Value != nil && t.Value.Kind() == constant.String {
			return []string{t.Value.ExactString()}, true
		}
	case *ssa.BinOp:
		if t.Op == token.ADD {
			if x, ok := ConstLikeStringValues(t.X); ok {
				if y, ok := ConstLikeStringValues(t.Y); ok {
					res := make([]string, 0, len(x)*len(y))
					for _, xx := range x {
						for _, yy := range y {
							xx, err := Unquote(xx)
							if err != nil {
								continue
							}
							yy, err := Unquote(yy)
							if err != nil {
								continue
							}
							res = append(res, xx+yy)
						}
					}
					if len(res) > 0 {
						return res, true
					}
				}
			}
		}
	case *ssa.Phi:
		res := make([]string, 0, len(t.Edges))
		for _, edge := range t.Edges {
			if c, ok := ConstLikeStringValues(edge); ok {
				res = append(res, c...)
			}
		}
		if len(res) > 0 {
			return res, true
		}
	case *ssa.Call:
		common := t.Common()
		if cvFn, ok := common.Value.(*ssa.Function); ok {
			if cvFn.Pkg != nil && cvFn.Pkg.Pkg.Path() == "fmt" && cvFn.Name() == "Sprintf" {
				fs, ok := ConstLikeStringValues(t.Call.Args[0])
				if !ok {
					break
				}
				if len(fs) != 1 {
					break
				}
				f := fs[0]
				f, err := Unquote(f)
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
