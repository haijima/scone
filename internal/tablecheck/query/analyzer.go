package query

import (
	"crypto/sha1"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer is ...
var Analyzer = &analysis.Analyzer{
	Name: "extractquery",
	Doc:  "tablecheck is ...",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		return ExtractQuery(ssaProg, &QueryOption{})
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(new(Result)),
}

type Result struct {
	Queries []*Query
}

type QueryOption struct {
	ExcludeQueries      []string
	ExcludePackages     []string
	ExcludePackagePaths []string
	ExcludeFiles        []string
	ExcludeFunctions    []string
	ExcludeQueryTypes   []string
	ExcludeTables       []string
	FilterQueries       []string
	FilterPackages      []string
	FilterPackagePaths  []string
	FilterFiles         []string
	FilterFunctions     []string
	FilterQueryTypes    []string
	FilterTables        []string
}

func ExtractQuery(ssaProg *buildssa.SSA, opt *QueryOption) (*Result, error) {
	foundQueries := make([]*Query, 0)
	for _, member := range ssaProg.SrcFuncs {
		foundQueries = append(foundQueries, analyzeFunc(ssaProg.Pkg, member, []token.Pos{}, opt)...)
		//foundQueries = append(foundQueries, analyzeFuncByAst(member, opt)...)
	}

	slices.SortFunc(foundQueries, func(a, b *Query) int {
		if a.Position().Offset == b.Position().Offset {
			if a.Raw == b.Raw {
				return 0
			}
			if a.Raw < b.Raw {
				return -1
			}
			return 1
		}
		if a.Position().Offset < b.Position().Offset {
			return -1
		}
		return 1
	})
	foundQueries = slices.CompactFunc(foundQueries, func(a, b *Query) bool {
		return a.Raw == b.Raw && a.Position().Offset == b.Position().Offset
	})
	return &Result{Queries: foundQueries}, nil
}

func analyzeFunc(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
	foundQueries := make([]*Query, 0)
	for _, block := range fn.Blocks {
		for _, instr := range block.Instrs {
			foundQueries = append(foundQueries, analyzeInstr(pkg, instr, append([]token.Pos{fn.Pos()}, pos...), opt)...)
		}
	}
	for _, anon := range fn.AnonFuncs {
		foundQueries = append(foundQueries, analyzeFunc(pkg, anon, append([]token.Pos{anon.Pos(), fn.Pos()}, pos...), opt)...)
	}
	return foundQueries
}

func analyzeInstr(pkg *ssa.Package, instr ssa.Instruction, pos []token.Pos, opt *QueryOption) []*Query {
	foundQueries := make([]*Query, 0)
	switch i := instr.(type) {
	case *ssa.Call:
		foundQueries = append(foundQueries, callToQueries(pkg, i, instr.Parent(), pos, opt)...)
	case *ssa.Phi:
		foundQueries = append(foundQueries, phiToQueries(pkg, i, instr.Parent(), pos, opt)...)
	}
	return foundQueries
}

func callToQueries(pkg *ssa.Package, i *ssa.Call, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
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

func phiToQueries(pkg *ssa.Package, a *ssa.Phi, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
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

func constToQuery(pkg *ssa.Package, a *ssa.Const, fn *ssa.Function, pos []token.Pos, opt *QueryOption) (*Query, bool) {
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

func analyzeFuncByAst(pkg *ssa.Package, fn *ssa.Function, pos []token.Pos, opt *QueryOption) []*Query {
	foundQueries := make([]*Query, 0)
	ast.Inspect(fn.Syntax(), func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if q, ok := toSqlQuery(lit.Value); ok {
				q.Func = fn
				q.Pos = append([]token.Pos{lit.Pos()}, pos...)
				q.Package = pkg
				if filter(q, opt) {
					foundQueries = append(foundQueries, q)
				}
			}
		}
		return true
	})
	return foundQueries
}

var SelectPattern = regexp.MustCompile("^(?i)(SELECT .+? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var JoinPattern = regexp.MustCompile("(?i)(JOIN `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?(?:(?: as)? [a-z0-9_]+)? (?:ON|USING)?)")
var SubQueryPattern = regexp.MustCompile("(?i)(SELECT .+? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var InsertPattern = regexp.MustCompile("^(?i)(INSERT(?: IGNORE)?(?: INTO)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var UpdatePattern = regexp.MustCompile("^(?i)(UPDATE(?: IGNORE)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`? SET)")
var DeletePattern = regexp.MustCompile("^(?i)(DELETE(?: IGNORE)? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")

func toSqlQuery(str string) (*Query, bool) {
	str, err := normalize(str)
	if err != nil {
		return nil, false
	}

	q := &Query{Raw: str}
	if matches := SelectPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Select
		q.Tables = make([]string, 0)
		if SubQueryPattern.MatchString(str) {
			for _, m := range SubQueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
		if JoinPattern.MatchString(str) {
			for _, m := range JoinPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
	} else if matches := InsertPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Insert
		q.Tables = []string{InsertPattern.FindStringSubmatch(str)[2]}
		if SubQueryPattern.MatchString(str) {
			for _, m := range SubQueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
	} else if matches := UpdatePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Update
		q.Tables = []string{UpdatePattern.FindStringSubmatch(str)[2]}
		if SubQueryPattern.MatchString(str) {
			for _, m := range SubQueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
	} else if matches := DeletePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Delete
		q.Tables = []string{DeletePattern.FindStringSubmatch(str)[2]}
		if SubQueryPattern.MatchString(str) {
			for _, m := range SubQueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
	} else {
		//slog.Warn(fmt.Sprintf("unknown query: %s", str))
		return nil, false
	}
	return q, true
}

func normalize(str string) (string, error) {
	str, err := strconv.Unquote(str)
	if err != nil {
		return str, err
	}
	str = strings.ReplaceAll(str, "\n", " ")
	str = strings.Join(strings.Fields(str), " ") // remove duplicate spaces
	str = strings.Trim(str, " ")
	str = strings.ToLower(str)
	return str, nil
}

func filter(q *Query, opt *QueryOption) bool {
	pkgName := q.Package.Pkg.Name()
	pkgPath := q.Package.Pkg.Path()
	file := q.Position().Filename
	funcName := q.Func.Name()
	queryType := q.Kind.String()
	table := q.Tables[0]
	h := sha1.New()
	h.Write([]byte(q.Raw))
	hash := fmt.Sprintf("%x", h.Sum(nil))

	return filterAndExclude(pkgName, opt.FilterPackages, opt.ExcludePackages) &&
		filterAndExclude(pkgPath, opt.FilterPackagePaths, opt.ExcludePackagePaths) &&
		filterAndExclude(file, opt.FilterFiles, opt.ExcludeFiles) &&
		filterAndExclude(funcName, opt.FilterFunctions, opt.ExcludeFunctions) &&
		filterAndExclude(queryType, opt.FilterQueryTypes, opt.ExcludeQueryTypes) &&
		filterAndExclude(table, opt.FilterTables, opt.ExcludeTables) &&
		filterAndExcludeFunc(hash, opt.FilterQueries, opt.ExcludeQueries, strings.HasPrefix)
}

func filterAndExclude(target string, filters []string, excludes []string) bool {
	return filterAndExcludeFunc(target, filters, excludes, func(target, input string) bool { return target == input })
}

func filterAndExcludeFunc(target string, filters []string, excludes []string, fn func(target, input string) bool) bool {
	match := true
	if filters != nil && len(filters) > 0 {
		match = false
		for _, f := range filters {
			if fn(target, f) {
				match = true
			}
		}
	}
	if excludes != nil && len(excludes) > 0 {
		for _, e := range excludes {
			if fn(target, e) {
				match = false
			}
		}
	}
	return match
}
