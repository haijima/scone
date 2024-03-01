package analysisutil

import (
	"go/ast"
	"go/constant"
	"go/token"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func GetPosition(pkg *ssa.Package, pos []token.Pos) token.Position {
	if pkg == nil || pkg.Prog == nil || pkg.Prog.Fset == nil {
		return token.Position{}
	}
	if i := slices.IndexFunc(pos, func(p token.Pos) bool { return p.IsValid() }); i > -1 {
		return pkg.Prog.Fset.Position(pos[i])
	}
	return pkg.Prog.Fset.Position(token.NoPos)
}

func ValueToStrings(v ssa.Value) ([]string, bool) {
	return valueToStrings(v, 0)
}

func valueToStrings(v ssa.Value, depth int) ([]string, bool) {
	if depth > 10 {
		slog.Debug("valueToStrings: too deep", slog.Any("value", v))
		return []string{}, false
	}
	depth++
	switch t := v.(type) {
	case *ssa.Const:
		return constToStrings(t)
	case *ssa.BinOp:
		return binOpToStrings(t, depth)
	case *ssa.Phi:
		return phiToStrings(t, depth)
	case *ssa.Call:
		if IsFunc(t.Common(), "fmt", "Sprintf") {
			return fmtSprintfToStrings(t, depth)
		} else if IsFunc(t.Common(), "strings", "Join") {
			return stringsJoinToStrings(t, depth)
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

func binOpToStrings(t *ssa.BinOp, depth int) ([]string, bool) {
	x, xok := valueToStrings(t.X, depth)
	y, yok := valueToStrings(t.Y, depth)
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

func phiToStrings(t *ssa.Phi, depth int) ([]string, bool) {
	res := make([]string, 0, len(t.Edges))
	for _, edge := range t.Edges {
		if s, ok := valueToStrings(edge, depth); ok {
			res = slices.Concat(res, s)
		}
	}
	return res, len(res) > 0
}

var fmtVerbRegexp = regexp.MustCompile(`(^|[^%]|(?:%%)+)(%(?:-?\d+|\+|#)?)(\w)`)

// fmtSprintfToStrings returns the possible string values of fmt.Sprintf.
func fmtSprintfToStrings(t *ssa.Call, depth int) ([]string, bool) {
	fs, ok := valueToStrings(t.Call.Args[0], depth)
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
func stringsJoinToStrings(t *ssa.Call, depth int) ([]string, bool) {
	// strings.Join
	joiner, ok := valueToStrings(t.Call.Args[1], depth)
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
	for _, c := range []uint8{'`', '"', '\''} {
		if len(str) >= 2 && str[0] == c && str[len(str)-1] == c {
			return strconv.Unquote(str)
		}
	}
	return str, nil
}

type FuncInfo struct {
	PkgPath  string
	RcvName  string
	FuncName string
}

func (f *FuncInfo) String() string {
	pkgPath := f.PkgPath
	if pkgPath != "" {
		pkgPath += "."
	}
	if f.RcvName != "" {
		return "(" + pkgPath + f.RcvName + ")." + f.FuncName
	}
	return pkgPath + f.FuncName
}

func IsFunc(common *ssa.CallCommon, pkgPath, funcName string) bool {
	if funcInfo, ok := GetFuncInfo(common); ok {
		return funcInfo.PkgPath == pkgPath && funcInfo.FuncName == funcName
	}
	return false
}

func GetFuncInfo(common *ssa.CallCommon) (funcInfo *FuncInfo, ok bool) {
	if common.IsInvoke() {
		// dynamic method call
		// e.g.
		// func Something(s fmt.Stringer) {
		//     s.String() // <--- s.String() is dynamic method call
		// }
		// or
		// func another(err error) {
		//     err.Error() // <--- err.Error() is built-in dynamic method call
		rcvName := common.Value.Type().String()
		var pkgPath string
		if common.Method.Pkg() != nil {
			pkgPath = common.Method.Pkg().Path()
			rcvName = strings.TrimPrefix(rcvName, pkgPath+".")
		}
		return &FuncInfo{PkgPath: pkgPath, RcvName: rcvName, FuncName: common.Method.Name()}, true
	} else {
		switch fn := common.Value.(type) {
		case *ssa.Builtin:
			// built-in function call
			// e.g. len, append, etc.
			return &FuncInfo{PkgPath: "", RcvName: "", FuncName: fn.Name()}, true
		case *ssa.MakeClosure:
			// static function closure call
			// e.g. func() { ... }()
			// names are described as xxxFunc$1()
			var pkgPath, funcName string
			if fn.Parent() != nil {
				pkgPath = fn.Parent().Package().Pkg.Name()
			}
			if fn.Fn != nil {
				funcName = fn.Fn.Name()
			}
			return &FuncInfo{PkgPath: pkgPath, RcvName: "", FuncName: funcName}, pkgPath != "" || funcName != ""
		case *ssa.Function:
			if fn.Signature.Recv() != nil {
				if fn.Signature.Recv().Pkg() != nil {
					// static method call
					// e.g.
					// foo := GetFoo()
					// foo.Bar() // <--- foo.Bar() is static method call
					rcvName := fn.Signature.Recv().Type().String()
					if i := strings.LastIndex(rcvName, "."); i > -1 {
						rcvName = rcvName[i+1:]
					}
					return &FuncInfo{PkgPath: fn.Signature.Recv().Pkg().Path(), RcvName: rcvName, FuncName: fn.Name()}, true
				} else {
					// unreachable: no builtin struct
					return &FuncInfo{PkgPath: "", RcvName: "", FuncName: fn.Name()}, true
				}
			} else {
				if fn.Pkg != nil {
					// static function call
					return &FuncInfo{PkgPath: fn.Pkg.Pkg.Path(), RcvName: "", FuncName: fn.Name()}, true
				} else if fn.Origin() != nil && fn.Origin().Pkg != nil {
					// generics static function call
					return &FuncInfo{PkgPath: fn.Origin().Pkg.Pkg.Path(), RcvName: "", FuncName: fn.Origin().Name()}, true
				} else {
					// unreachable?
					return &FuncInfo{PkgPath: "", RcvName: "", FuncName: fn.Name()}, true
				}
			}
		case *ssa.Parameter:
			// dynamic function call
			// e.g.
			// func foo(fn func() string) {
			//     s := fn() // <--- fn() is dynamic function call
			//     fmt.Println(s)
			// }
			return &FuncInfo{PkgPath: "", RcvName: "", FuncName: fn.Name()}, true
		case *ssa.Call:
			// dynamic function call
			// e.g.
			// func foo() {
			//     c := getCallable()
			//     fmt.Println(c())  // <--- c() is dynamic function call
			// }
			return &FuncInfo{PkgPath: "", RcvName: "", FuncName: fn.Call.Value.Name()}, true
		default:
			// dynamic function call
			return &FuncInfo{PkgPath: "", RcvName: "", FuncName: fn.Name()}, true
		}
	}
}

func InstrToCallCommon(instr ssa.Instruction) (*ssa.CallCommon, bool) {
	switch i := instr.(type) {
	case ssa.CallInstruction:
		return i.Common(), true
	case *ssa.Extract:
		if call, ok := i.Tuple.(*ssa.Call); ok {
			return call.Common(), true
		}
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
