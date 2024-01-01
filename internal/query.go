package internal

import (
	"go/ast"
	"go/token"
	"reflect"
	"regexp"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// ExtractQueryAnalyzer is ...
var ExtractQueryAnalyzer = &analysis.Analyzer{
	Name: "extractquery",
	Doc:  "tablecheck is ...",
	Run:  extractQuery,
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(new(queryResult)),
}

type queryResult struct {
	queries []*query
}

type query struct {
	kind   string
	fn     *ssa.Function
	name   string
	raw    string
	tables []string
}

func extractQuery(pass *analysis.Pass) (any, error) {
	ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)

	foundQueries := make([]*query, 0)
	for _, member := range ssaProg.SrcFuncs {
		ast.Inspect(member.Syntax(), func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if q, ok := toSqlQuery(lit.Value); ok {
					q.fn = member
					q.name = member.Name()
					foundQueries = append(foundQueries, q)
				}
			}
			return true
		})
	}

	return &queryResult{queries: foundQueries}, nil
}

var sqlPattern = regexp.MustCompile(`(?i)(SELECT .+ FROM|INSERT INTO|DELETE FROM|UPDATE) ([a-zA-Z0-9_]+)`)
var selectPattern = regexp.MustCompile(`(?i)(SELECT .+ FROM) ([a-zA-Z0-9_]+)`)
var insertPattern = regexp.MustCompile(`(?i)(INSERT INTO) ([a-zA-Z0-9_]+)`)
var updatePattern = regexp.MustCompile(`(?i)(DELETE FROM) ([a-zA-Z0-9_]+)`)
var deletePattern = regexp.MustCompile(`(?i)(UPDATE) ([a-zA-Z0-9_]+)`)

func toSqlQuery(str string) (*query, bool) {
	if !sqlPattern.MatchString(str) {
		return nil, false
	}

	q := &query{
		//fn:     member,
		//name:   member.Name(),
		raw:    str,
		tables: sqlPattern.FindStringSubmatch(str)[2:],
	}

	if matches := selectPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = "SELECT"
	} else if matches := insertPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = "INSERT"
	} else if matches := updatePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = "UPDATE"
	} else if matches := deletePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = "DELETE"
	} else {
		return nil, false
	}
	return q, true
}
