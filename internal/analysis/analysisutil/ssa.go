package analysisutil

import (
	"go/ast"
	"go/constant"
	"go/token"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func GetPosition(pkg *ssa.Package, pos []token.Pos) token.Position {
	if i := slices.IndexFunc(pos, func(p token.Pos) bool { return p.IsValid() }); i > -1 {
		return pkg.Prog.Fset.Position(pos[i])
	}
	return pkg.Prog.Fset.Position(token.NoPos)
}

func ConstLikeStringValues(v ssa.Value) ([]string, bool) {
	switch t := v.(type) {
	case *ssa.Const:
		return constToStrings(t)
	case *ssa.BinOp:
		return binOpToStrings(t)
	case *ssa.Phi:
		return phiToStrings(t)
	case *ssa.Call:
		if IsFunc(t.Common(), "fmt", "Sprintf") {
			return fmtSprintfToStrings(t)
		} else if IsFunc(t.Common(), "strings", "Join") {
			return stringsJoinToStrings(t)
		}
	}
	return []string{}, false
}

func constToStrings(t *ssa.Const) ([]string, bool) {
	if t.Value != nil && t.Value.Kind() == constant.String {
		if s, err := Unquote(t.Value.ExactString()); err == nil {
			return []string{s}, true
		}
	}
	return []string{}, false
}

func binOpToStrings(t *ssa.BinOp) ([]string, bool) {
	x, xok := ConstLikeStringValues(t.X)
	y, yok := ConstLikeStringValues(t.Y)
	if xok && yok && len(x) > 0 && len(y) > 0 && t.Op == token.ADD {
		res := make([]string, 0, len(x)*len(y))
		for _, xx := range x {
			for _, yy := range y {
				res = append(res, xx+yy)
			}
		}
		return res, true
	}
	return []string{}, false
}

func phiToStrings(t *ssa.Phi) ([]string, bool) {
	res := make([]string, 0, len(t.Edges))
	for _, edge := range t.Edges {
		if s, ok := ConstLikeStringValues(edge); ok {
			res = append(res, s...)
		}
	}
	return res, len(res) > 0
}

var fmtVerbRegexp = regexp.MustCompile(`(^|[^%]|(?:%%)+)(%(?:-?\d+|\+|#)?)(\w)`)

// fmtSprintfToStrings returns the possible string values of fmt.Sprintf.
func fmtSprintfToStrings(t *ssa.Call) ([]string, bool) {
	fs, ok := ConstLikeStringValues(t.Call.Args[0])
	if !ok && len(fs) == 1 {
		return []string{}, false
	}
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
	if !fmtVerbRegexp.MatchString(f) { // no more verbs
		return []string{f}, true
	}
	return []string{}, false
}

// stringsJoinToStrings returns the possible string values of strings.Join.
func stringsJoinToStrings(t *ssa.Call) ([]string, bool) {
	// strings.Join
	joiner, ok := ConstLikeStringValues(t.Call.Args[1])
	if !ok || len(joiner) != 1 {
		return []string{}, false
	}
	firstArg := t.Call.Args[0]
	astArgs := make([]string, 0)
	ast.Inspect(t.Parent().Syntax(), func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if cl, ok := n.(*ast.CompositeLit); ok && n.Pos() <= firstArg.Pos() && firstArg.Pos() < n.End() {
			for _, elt := range cl.Elts {
				if bl, ok := elt.(*ast.BasicLit); ok {
					if unquoted, err := Unquote(bl.Value); err == nil {
						astArgs = append(astArgs, unquoted)
					}
				}
			}
			if len(astArgs) != len(cl.Elts) {
				// not all elements are constant or some elements are failed to unquote
				astArgs = []string{}
			}
			return false
		}
		return true
	})
	if len(astArgs) > 0 {
		return []string{strings.Join(astArgs, joiner[0])}, true
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

func IsFunc(common *ssa.CallCommon, pkgPath, funcName string) bool {
	if path, name, ok := GetFuncInfo(common); ok {
		return path == pkgPath && name == funcName
	}
	return false
}

func GetFuncInfo(common *ssa.CallCommon) (pkgPath, funcName string, ok bool) {
	switch fn := common.Value.(type) {
	case *ssa.Builtin:
		// built-in function call
		//fmt.Fprintf(os.Stderr, "built-in function call: %v\n", fn)
	case *ssa.MakeClosure:
		// static function closure call
		//fmt.Fprintf(os.Stderr, "static function closure call: %v\n", fn)
	case *ssa.Function:
		if fn.Signature.Recv() != nil {
			if fn.Signature.Recv().Pkg() != nil {
				// static method call
				//fmt.Fprintf(os.Stderr, "static method call: %v\n", fn)
				return fn.Signature.Recv().Pkg().Path(), fn.Name(), true
			} else {
				// builtin?
				//fmt.Fprintf(os.Stderr, "static method call(%s) should have package\n", fn.Name())
			}
		} else {
			if fn.Pkg != nil {
				// static function call
				//fmt.Fprintf(os.Stderr, "static function call: %v\n", fn)
				return fn.Pkg.Pkg.Path(), fn.Name(), true
			} else if fn.Origin() != nil && fn.Origin().Pkg != nil {
				// generics?
				// static function call
				//fmt.Fprintf(os.Stderr, "static function call: %v\n", fn)
				return fn.Origin().Pkg.Pkg.Path(), fn.Origin().Name(), true
			}
		}
	default:
		if common.IsInvoke() {
			if common.Method.Pkg() != nil {
				// dynamic method call
				//fmt.Fprintf(os.Stderr, "dynamic method call: %v\n", common.Method)
				return common.Method.Pkg().Path(), common.Method.Name(), true
			} else {
				// builtin dynamic method call
				//fmt.Fprintf(os.Stderr, "builtin dynamic method call: %v\n", common.Method)
			}
		} else {
			// dynamic function call
			//fmt.Fprintf(os.Stderr, "dynamic function call: %v\n", fn)
		}
	}
	return "", "", false // Can't get package name of the function
}

func InstrToCallCommon(instr ssa.Instruction) (*ssa.CallCommon, bool) {
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

func ValueToCallCommon(value ssa.Value) (*ssa.CallCommon, bool) {
	switch v := value.(type) {
	case *ssa.Call:
		return v.Common(), true
	case *ssa.Extract:
		if call, ok := v.Tuple.(*ssa.Call); ok {
			return call.Common(), true
		}
	}
	return nil, false
}
