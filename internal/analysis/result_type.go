package analysis

import (
	"go/token"
	"go/types"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/analysisutil/ssautil"
	"github.com/haijima/scone/internal/sql"
	"golang.org/x/exp/maps"
	"golang.org/x/tools/go/ssa"
)

type QueryResults []*QueryResult

func (qrs QueryResults) AllTables() []*sql.Table {
	s := maps.Values(qrs.allTableMap())
	slices.SortFunc(s, func(a, b *sql.Table) int { return strings.Compare(a.Name, b.Name) })
	return s
}

func (qrs QueryResults) AllTableNames() []string {
	return mapset.Sorted(mapset.NewSetFromMapKeys(qrs.allTableMap()))
}

func (qrs QueryResults) allTableMap() map[string]*sql.Table {
	qgs := make([]*sql.QueryGroup, 0)
	for _, qr := range qrs {
		qgs = append(qgs, qr.QueryGroup)
	}
	return sql.QueryGroups(qgs).AllTableMap()
}

type QueryResult struct {
	*sql.QueryGroup
	Meta *Meta
}

func NewQueryResult(meta *Meta) *QueryResult {
	return &QueryResult{QueryGroup: sql.NewQueryGroup(), Meta: meta}
}

func (qr *QueryResult) Append(qs ...*sql.Query) {
	if qr.QueryGroup == nil {
		qr.QueryGroup = sql.NewQueryGroup()
	} else if qr.List == nil {
		qr.List = mapset.NewSet[*sql.Query]()
	}
	qr.List.Append(qs...)
}

func (qr *QueryResult) Compare(other *QueryResult) int {
	if !qr.Meta.Equal(other.Meta) {
		return qr.Meta.Compare(other.Meta)
	}
	return slices.CompareFunc(qr.Queries(), other.Queries(), func(a, b *sql.Query) int { return strings.Compare(a.Hash(), b.Hash()) })
}

type Meta struct {
	Func        *ssa.Function
	Pos         []token.Pos
	FromComment bool
}

func NewMeta(fn *ssa.Function, pos ...token.Pos) *Meta {
	return &Meta{Func: fn, Pos: pos}
}

func (m *Meta) Package() *types.Package {
	//return m.pkg.Pkg

	if m.Func == nil || m.Func.Pkg == nil {
		return &types.Package{}
	}
	return m.Func.Pkg.Pkg
}

var pathDirRegex = regexp.MustCompile(`([^/]+)/`)

func (m *Meta) PackagePath(abbreviate bool) string {
	if abbreviate {
		return pathDirRegex.ReplaceAllStringFunc(m.Package().Path(), func(m string) string { return m[:1] + "/" })
	}
	return m.Package().Path()
}

func (m *Meta) Position() token.Position {
	if m.Func == nil {
		return token.Position{}
	}
	return ssautil.GetPosition(m.Func.Pkg, m.Pos...)
}

func (m *Meta) FLC() string {
	return filepath.Base(m.Position().String())
}

func (m *Meta) Compare(other *Meta) int {
	if m.Package().Path() != other.Package().Path() {
		return strings.Compare(m.Package().Path(), other.Package().Path())
	} else if m.Position().Filename != other.Position().Filename {
		return strings.Compare(m.Position().Filename, other.Position().Filename)
	} else if m.Position().Offset != other.Position().Offset {
		return m.Position().Offset - other.Position().Offset
	}
	return 0
}

func (m *Meta) Equal(other *Meta) bool {
	return m.Compare(other) == 0
}

func (m *Meta) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("package", m.PackagePath(true)),
		slog.String("file", m.FLC()),
		slog.String("function", m.Func.Name()),
	)
}
