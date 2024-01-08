package query

import (
	"go/ast"
	"go/token"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

// Analyzer is ...
var Analyzer = &analysis.Analyzer{
	Name: "extractquery",
	Doc:  "tablecheck is ...",
	Run: func(pass *analysis.Pass) (interface{}, error) {
		ssaProg := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
		return ExtractQuery(ssaProg)
	},
	Requires: []*analysis.Analyzer{
		buildssa.Analyzer,
	},
	ResultType: reflect.TypeOf(new(Result)),
}

type Result struct {
	Queries        []*Query
	Tables         []string
	QueriesByTable map[string][]*Query
}

func ExtractQuery(ssaProg *buildssa.SSA) (*Result, error) {
	foundQueries := make([]*Query, 0)
	foundTableMap := make(map[string]any)
	queriesByTable := make(map[string][]*Query)
	for _, member := range ssaProg.SrcFuncs {
		ast.Inspect(member.Syntax(), func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if q, ok := toSqlQuery(lit.Value); ok {
					q.Func = member
					q.Name = member.Name()
					foundQueries = append(foundQueries, q)
					for _, t := range q.Tables {
						foundTableMap[t] = struct{}{}
					}
					for _, t := range q.Tables {
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
	return &Result{Queries: foundQueries, Tables: foundTables, QueriesByTable: queriesByTable}, nil
}

var selectPattern = regexp.MustCompile(`^(?i)(SELECT .+ FROM) ([a-z0-9_]+)`)
var joinPattern = regexp.MustCompile(`(?i)(?:JOIN ([a-z0-9_]+) (?:[a-z0-9_]+ )?ON)+`)
var subqueryPattern = regexp.MustCompile(`(?i)(SELECT .+ FROM) ([a-z0-9_]+)`)
var insertPattern = regexp.MustCompile(`^(?i)(INSERT INTO) ([a-z0-9_]+)`)
var updatePattern = regexp.MustCompile(`^(?i)(UPDATE) ([a-z0-9_]+)`)
var deletePattern = regexp.MustCompile(`^(?i)(DELETE FROM) ([a-z0-9_]+)`)

func toSqlQuery(str string) (*Query, bool) {
	str, err := normalize(str)
	if err != nil {
		return nil, false
	}

	q := &Query{Raw: str}
	if matches := selectPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Select
		//q.Tables = sqlPattern.FindStringSubmatch(str)[2:]
		q.Tables = make([]string, 0)
		if subqueryPattern.MatchString(str) {
			for _, m := range subqueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
		if joinPattern.MatchString(str) {
			for _, m := range joinPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[1])
			}
		}
	} else if matches := insertPattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Insert
		q.Tables = insertPattern.FindStringSubmatch(str)[2:]
		if subqueryPattern.MatchString(str) {
			for _, m := range subqueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
	} else if matches := updatePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Update
		q.Tables = updatePattern.FindStringSubmatch(str)[2:]
		if subqueryPattern.MatchString(str) {
			for _, m := range subqueryPattern.FindAllStringSubmatch(str, -1) {
				q.Tables = append(q.Tables, m[2])
			}
		}
	} else if matches := deletePattern.FindStringSubmatch(str); len(matches) > 2 {
		q.Kind = Delete
		q.Tables = deletePattern.FindStringSubmatch(str)[2:]
		if subqueryPattern.MatchString(str) {
			for _, m := range subqueryPattern.FindAllStringSubmatch(str, -1) {
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
