package query

import (
	"crypto/sha1"
	"fmt"
	"go/token"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

type Query struct {
	Kind    QueryKind
	Func    *ssa.Function
	Pos     []token.Pos
	Package *ssa.Package
	Raw     string
	Tables  []string
}

func (q *Query) Position() token.Position {
	return GetPosition(q.Package, q.Pos)
}

func (q *Query) Sha() string {
	h := sha1.New()
	h.Write([]byte(q.Raw))
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

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

type QueryKind int

const (
	Unknown QueryKind = iota
	Select
	Insert
	Delete
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
	str, err := unquote(str)
	if err != nil {
		return str, err
	}
	str = strings.ReplaceAll(str, "\n", " ")
	str = strings.Join(strings.Fields(str), " ") // remove duplicate spaces
	str = strings.Trim(str, " ")
	str = strings.ToLower(str)
	return str, nil
}

func unquote(str string) (string, error) {
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
