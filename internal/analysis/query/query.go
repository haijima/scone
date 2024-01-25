package query

import (
	"crypto/sha1"
	"fmt"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	"golang.org/x/tools/go/ssa"

	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

type Query struct {
	Kind            QueryKind
	Func            *ssa.Function
	Pos             []token.Pos
	Package         *ssa.Package
	Raw             string
	MainTable       string
	Tables          []string
	FilterColumnMap map[string]mapset.Set[string]
}

func (q *Query) Position() token.Position {
	return analysisutil.GetPosition(q.Package, q.Pos)
}

func (q *Query) Sha() string {
	h := sha1.New()
	h.Write([]byte(q.Raw))
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

func (q *Query) FLC() string {
	return fmt.Sprintf("%s:%d:%d", filepath.Base(q.Position().Filename), q.Position().Line, q.Position().Column)
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

func (k QueryKind) ColoredString() string {
	return k.Color(k.String())
}

func (k QueryKind) Color(str string) string {
	switch k {
	case Select:
		return color.BlueString(str)
	case Insert:
		return color.GreenString(str)
	case Delete:
		return color.RedString(str)
	case Replace:
		return color.YellowString(str)
	case Update:
		return color.YellowString(str)
	default:
		return color.HiBlackString(str)
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

var namedParameterRegexp = regexp.MustCompile(`(?i):[a-z_]+`)
var trailingCommentRegexp = regexp.MustCompile(`(?i)--.*\r?\n`)

func normalize(str string) (string, error) {
	str, err := analysisutil.Unquote(str)
	if err != nil {
		return str, err
	}
	str = namedParameterRegexp.ReplaceAllString(str, "?")  // replace named parameters with parameter of prepared statement
	str = trailingCommentRegexp.ReplaceAllString(str, " ") // remove comments and join lines
	str = strings.Replace(str, "\t", " ", -1)              // remove tabs
	str = strings.Join(strings.Fields(str), " ")           // remove duplicate spaces
	str = strings.TrimSpace(str)                           // remove leading and trailing spaces

	return str, nil
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
