package query

import (
	"crypto/sha1"
	"fmt"
	"go/token"
	"log/slog"
	"regexp"
	"strings"

	"github.com/haijima/scone/internal/analysis/analysisutil"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"golang.org/x/tools/go/ssa"

	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

type Query struct {
	Kind      QueryKind
	Func      *ssa.Function
	Pos       []token.Pos
	Package   *ssa.Package
	Raw       string
	MainTable string
	Tables    []string
}

func (q *Query) Position() token.Position {
	return analysisutil.GetPosition(q.Package, q.Pos)
}

func (q *Query) Sha() string {
	h := sha1.New()
	h.Write([]byte(q.Raw))
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

type QueryKind int

const (
	Unknown QueryKind = iota
	Select
	Insert
	Delete
	Replace
	Update
)

func (k QueryKind) String() string {
	switch k {
	case Select:
		return "SELECT"
	case Insert:
		return "INSERT"
	case Delete:
		return "DELETE"
	case Replace:
		return "REPLACE"
	case Update:
		return "UPDATE"
	default:
		return "UNKNOWN"
	}
}

var SelectPattern = regexp.MustCompile("^(?i)(SELECT .+? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var JoinPattern = regexp.MustCompile("(?i)(JOIN `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?(?:(?: as)? [a-z0-9_]+)? (?:ON|USING)?)")
var SubQueryPattern = regexp.MustCompile("(?i)(SELECT .+? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var InsertPattern = regexp.MustCompile("^(?i)(INSERT(?: IGNORE)?(?: INTO)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var UpdatePattern = regexp.MustCompile("^(?i)(UPDATE(?: IGNORE)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?.* SET)")
var ReplacePattern = regexp.MustCompile("^(?i)(REPLACE(?: INTO)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var DeletePattern = regexp.MustCompile("^(?i)(DELETE(?: IGNORE)? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")

func toSqlQuery(str string) (*Query, bool) {
	str, err := normalize(str)
	if err != nil {
		return nil, false
	}

	q, err := parse(str)
	if err != nil {
		return nil, false
	}
	return q, true
}

func normalize(str string) (string, error) {
	str, err := analysisutil.Unquote(str)
	if err != nil {
		return str, err
	}
	str = regexp.MustCompile(`(?i):[a-z_]+`).ReplaceAllString(str, "?") // replace named parameters with parameter of prepared statement
	return str, nil
}

type tableX struct {
	tableNames []string
}

func (v *tableX) Enter(in ast.Node) (ast.Node, bool) {
	if name, ok := in.(*ast.TableName); ok {
		v.tableNames = append(v.tableNames, name.Name.O)
	}
	return in, false
}

func (v *tableX) Leave(in ast.Node) (ast.Node, bool) {
	return in, true
}

func parse(sql string) (*Query, error) {
	p := parser.New()

	stmtNodes, warns, err := p.ParseSQL(sql)
	if err != nil {
		return nil, err
	}
	for _, w := range warns {
		slog.Warn(w.Error())
	}
	if len(stmtNodes) > 1 {
		slog.Warn("multiple statements in one query")
	}

	stmt := stmtNodes[0]
	q := &Query{Raw: stmt.Text()}

	v := &tableX{}
	stmt.Accept(v)
	tables := v.tableNames
	if len(tables) > 0 {
		q.MainTable = tables[0]
	}
	tableSet := make([]string, 0)
	seen := make(map[string]bool)
	for _, t := range tables {
		if !seen[t] {
			tableSet = append(tableSet, t)
			seen[t] = true
		}
	}
	q.Tables = tableSet // q.Tables[0] == q.MainTable

	switch s := stmt.(type) {
	case *ast.SelectStmt:
		q.Kind = Select
	case *ast.InsertStmt:
		if s.IsReplace {
			q.Kind = Replace
		} else {
			q.Kind = Insert
		}
	case *ast.UpdateStmt:
		q.Kind = Update
	case *ast.DeleteStmt:
		q.Kind = Delete
	default:
		q.Kind = Unknown
	}
	return q, nil
}

// reserved keywords in SQL
var keywords = []string{
	"SELECT", "FROM", "WHERE", "INSERT", "UPDATE", "DELETE", "JOIN",
	"COUNT", "GROUP", "BY", "HAVING", "ORDER", "LIMIT", "OFFSET",
	"INNER", "LEFT", "RIGHT", "FULL", "OUTER", "CROSS", "NATURAL",
	"UNION", "ALL", "AND", "OR", "NOT", "AS", "IN", "ON", "IS",
	"NULL", "LIKE", "EXISTS", "CASE", "WHEN", "THEN", "ELSE",
	"END", "CREATE", "ALTER", "DROP", "TABLE", "INDEX", "VIEW",
	"TRIGGER", "USE", "DATABASE", "PRIMARY", "KEY", "FOREIGN",
	"REFERENCES", "DISTINCT", "SET", "VALUES", "INTO", "PROCEDURE",
	"FUNCTION", "DECLARE", "CURSOR", "FETCH", "GRANT", "REVOKE",
	"BEGIN", "TRANSACTION", "COMMIT", "ROLLBACK", "SAVEPOINT",
	"LOCK", "UNLOCK", "MERGE", "EXCEPT", "INTERSECT", "MINUS",
	"DESC", "ASC", "BETWEEN", "TRUNCATE", "CAST", "CONVERT",
}
var reservedKeywordsRegexp = regexp.MustCompile(`(?i)\b(` + strings.Join(keywords, "|") + `)\b`)
var backQuoteRegexp = regexp.MustCompile("`(.+)`")

func convertSQLKeywordsToUpper(sql string) string {
	// convert reserved keywords to upper case
	sql = reservedKeywordsRegexp.ReplaceAllStringFunc(sql, strings.ToUpper)
	// convert table names to lower case
	sql = backQuoteRegexp.ReplaceAllStringFunc(sql, strings.ToLower)
	return sql
}
