package analysisutil

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestGetFuncInfo(t *testing.T) {

	tests := []struct {
		name         string
		targetFunc   string
		index        int
		wantPkgPath  string
		wantRcvName  string
		wantFuncName string
	}{
		{"dynamic method call", "dynamicMethod", 0, "fmt", "Stringer", "String"},
		{"built-in dynamic method call", "builtinDynamicMethod", 0, "", "error", "Error"},
		{"built-in function call", "builtinFunc", 0, "", "", "append"},
		{"static function closure call", "staticFuncClosure", 0, "main", "", "staticFuncClosure$1"},
		{"static method call", "staticMethod", 0, "github.com/haijima/scone/test", "Foo", "String"},
		{"static function call", "staticFunc", 0, "fmt", "", "Println"},
		{"anonymous static function call", "anonymousStaticFunc", 0, "github.com/haijima/scone/test", "", "anonymousStaticFunc$1"},
		{"generics static function call", "genericsStaticFunc", 0, "github.com/haijima/scone/test", "", "foo"},
		{"dynamic function call", "dynamicFuncCall", 0, "", "", "fn"},
		{"dynamic function call", "dynamicFuncCall2", 0, "", "", "callableVar"},
		{"dynamic function call", "dynamicFuncCall3", 1, "", "", "getCallable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, err := ReadAndGetCalls("./testdata/src/func_info.go", "github.com/haijima/scone/test", "main", tt.targetFunc)
			require.NoError(t, err)
			require.Truef(t, len(calls) > tt.index, "index out of range: %d", tt.index)

			info, ok := GetFuncInfo(calls[tt.index].Common())
			require.True(t, ok)
			assert.Equal(t, tt.wantPkgPath, info.PkgPath, "Package path mismatch: want %q, got %q", tt.wantPkgPath, info.PkgPath)
			assert.Equal(t, tt.wantRcvName, info.RcvName, "Receiver name mismatch: want %q, got %q", tt.wantRcvName, info.RcvName)
			assert.Equal(t, tt.wantFuncName, info.FuncName, "Function name mismatch: want %q, got %q", tt.wantFuncName, info.FuncName)
		})
	}
}

func ReadAndGetCalls(fileName, packagePath, packageName, targetFuncName string) ([]*ssa.Call, error) {
	source, err := os.ReadFile(fileName)
	if err != nil {
		return []*ssa.Call{}, err
	}
	return getCalls(source, fileName, packagePath, packageName, targetFuncName)
}

func getCalls(source []byte, fileName, packagePath, packageName, targetFuncName string) ([]*ssa.Call, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, fileName, source, parser.ParseComments)
	if err != nil {
		return []*ssa.Call{}, err
	}
	files := []*ast.File{f}
	pkg := types.NewPackage(packagePath, packageName)
	hello, _, err := ssautil.BuildPackage(&types.Config{Importer: importer.Default()}, fset, pkg, files, ssa.SanityCheckFunctions)
	if err != nil {
		return []*ssa.Call{}, err
	}
	calls := make([]*ssa.Call, 0)
	for _, member := range hello.Members {
		if fn, ok := member.(*ssa.Function); ok {
			if fn.Name() != targetFuncName {
				continue
			}
			for _, block := range fn.Blocks {
				for _, instr := range block.Instrs {
					if call, ok := instr.(*ssa.Call); ok {
						calls = append(calls, call)
					}
				}
			}
		}
	}
	return calls, nil
}
