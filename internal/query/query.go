package query

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
)

type QueryGroup struct {
	List mapset.Set[*Query]
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

func (q *Query) Sha() string {
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

func ParseString(str string) (*Query, bool) {
	str, err := Normalize(str)
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

func Normalize(str string) (string, error) {
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
