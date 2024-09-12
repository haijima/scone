package sql

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

type QueryGroups []*QueryGroup

func (qgs QueryGroups) AllTableMap() map[string]*Table {
	tables := map[string]*Table{}
	for _, qg := range qgs {
		for _, q := range qg.List.ToSlice() {
			for _, t := range q.Tables {
				if _, ok := tables[t]; !ok {
					tables[t] = NewTable(t)
					tables[t].filterColumns = nil
				}
				tables[t].kinds.Add(q.Kind)
			}
			for t, cols := range q.FilterColumnMap {
				//if _, ok := tables[t]; !ok {
				//	tables[t] = NewTable(t)
				//	tables[t].filterColumns = nil
				//}
				if tables[t].filterColumns == nil {
					tables[t].filterColumns = cols
				} else {
					tables[t].filterColumns = tables[t].filterColumns.Intersect(cols)
				}
			}
		}
	}
	return tables
}

type QueryGroup struct {
	List mapset.Set[*Query]
}

func (qg *QueryGroup) Queries() []*Query {
	if qg == nil || qg.List == nil {
		return []*Query{}
	}
	s := qg.List.ToSlice()
	slices.SortFunc(s, func(a, b *Query) int { return strings.Compare(a.Raw, b.Raw) })
	return s
}

func NewQueryGroup() *QueryGroup {
	return &QueryGroup{List: mapset.NewSet[*Query]()}
}

func NewQueryGroupFrom(queries ...*Query) *QueryGroup {
	qg := NewQueryGroup()
	for _, q := range queries {
		qg.List.Add(q)
	}
	return qg
}

type Query struct {
	Kind            QueryKind
	Raw             string
	MainTable       string
	Tables          []string
	FilterColumnMap map[string]mapset.Set[string]
}

func (q *Query) Hash() string {
	h := sha1.New()
	h.Write([]byte(q.Raw))
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

func (q *Query) String() string {
	ellipsis := q.Raw
	if len(ellipsis) > 60 {
		lastSpaceIx := -1
		for i, r := range ellipsis {
			if unicode.IsSpace(r) {
				lastSpaceIx = i
			}
			if i >= 60-4 && lastSpaceIx != -1 {
				ellipsis = ellipsis[:lastSpaceIx] + " ..."
				break
			}
		}
	}
	return ellipsis
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

func (k QueryKind) CRUD() string {
	switch k {
	case Insert:
		return "C"
	case Select:
		return "R"
	case Replace, Update:
		return "U"
	case Delete:
		return "D"
	default:
		return "?"
	}
}

func (k QueryKind) ColoredString() string {
	return k.Color(k.String()) //nolint:govet
}

func (k QueryKind) Color(format string, a ...interface{}) string {
	return color.New(k.ColorAttribute()).Sprintf(format, a...)
}

func (k QueryKind) ColorAttribute() color.Attribute {
	switch k {
	case Select:
		return color.FgBlue
	case Insert:
		return color.FgGreen
	case Delete:
		return color.FgRed
	case Replace, Update:
		return color.FgYellow
	default:
		return color.FgHiBlack
	}
}

func ParseString(str string) (*Query, bool) {
	str = Normalize(str)
	q, err := parse(str)
	if err != nil {
		return nil, false
	}
	return q, true
}

var namedParameterRegexp = regexp.MustCompile(`(?i):[a-z_]+`)
var trailingCommentRegexp = regexp.MustCompile(`(?i)--.*\r?\n`)

func Normalize(str string) string {
	str = namedParameterRegexp.ReplaceAllString(str, "?")  // replace named parameters with parameter of prepared statement
	str = trailingCommentRegexp.ReplaceAllString(str, " ") // remove comments and join lines
	str = strings.ReplaceAll(str, "\t", " ")               // remove tabs
	str = strings.Join(strings.Fields(str), " ")           // remove duplicate spaces
	str = strings.TrimSpace(str)                           // remove leading and trailing spaces
	return str
}
