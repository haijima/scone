package tablecheck

import (
	"go/ast"
	"go/token"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// ExtractQueryAnalyzer is ...
var ExtractQueryAnalyzer = &analysis.Analyzer{
	Name: "extractquery",
	Doc:  "tablecheck is ...",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		return ExtractQuery(ssaProg)
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(new(QueryResult)),
}

type QueryResult struct {
	queries        []*Query
	tables         []string
	queriesByTable map[string][]*Query
}

type Query struct {
	kind   QueryKind
	fn     *ssa.Function
	name   string
	raw    string
	tables []string
}

type QueryKind int

const (
	Unknown QueryKind = iota
	Select
	Insert
	Delete
	Update
)

func ExtractQuery(ssaProg *buildssa.SSA) (*QueryResult, error) {
	foundQueries := make([]*Query, 0)
	foundTableMap := make(map[string]any)
	queriesByTable := make(map[string][]*Query)
	for _, member := range ssaProg.SrcFuncs {
		ast.Inspect(member.Syntax(), func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if q, ok := toSqlQuery(lit.Value); ok {
					q.fn = member
					q.name = member.Name()
					foundQueries = append(foundQueries, q)
					for _, t := range q.tables {
						foundTableMap[t] = struct{}{}
					}
					for _, t := range q.tables {
						if _, ok := queriesByTable[t]; !ok {
							queriesByTable[t] = make([]*Query, 0)
						}
						queriesByTable[t] = append(queriesByTable[t], q)
					}
				}
			}
			return true
		})
	}

	foundTables := make([]string, 0)
	for t := range foundTableMap {
		foundTables = append(foundTables, t)
	}
	return &QueryResult{queries: foundQueries, tables: foundTables, queriesByTable: queriesByTable}, nil
}

var sqlPattern = regexp.MustCompile(`^(?i)(SELECT .+ FROM|INSERT INTO|UPDATE|DELETE FROM) ([a-zA-Z0-9_]+)`)
var selectPattern = regexp.MustCompile(`^(?i)(SELECT .+ FROM) ([a-zA-Z0-9_]+)`)
var insertPattern = regexp.MustCompile(`^(?i)(INSERT INTO) ([a-zA-Z0-9_]+)`)
var updatePattern = regexp.MustCompile(`^(?i)(UPDATE) ([a-zA-Z0-9_]+)`)
var deletePattern = regexp.MustCompile(`^(?i)(DELETE FROM) ([a-zA-Z0-9_]+)`)

func toSqlQuery(str string) (*Query, bool) {
	str, err := normalize(str)
	if err != nil {
		return nil, false
	}
	if !sqlPattern.MatchString(str) {
		return nil, false
	}

	q := &Query{
		raw:    str,
		tables: sqlPattern.FindStringSubmatch(str)[2:],
	}
	if matches := selectPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = Select
	} else if matches := insertPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = Insert
	} else if matches := updatePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = Update
	} else if matches := deletePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.kind = Delete
	} else {
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
	return str, nil
}
