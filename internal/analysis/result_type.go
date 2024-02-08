package analysis

import (
	"go/token"
	"log/slog"
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis/analysisutil"
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
	return slices.CompareFunc(qr.Queries(), other.Queries(), func(a, b *sql.Query) int { return strings.Compare(a.Sha(), b.Sha()) })
}

type Meta struct {
	Package *ssa.Package
	Func    *ssa.Function
	Pos     []token.Pos
}

func NewMeta(pkg *ssa.Package, fn *ssa.Function, pos token.Pos, fallbackPos ...token.Pos) *Meta {
	return &Meta{Package: pkg, Func: fn, Pos: append([]token.Pos{pos}, fallbackPos...)}
}

func (m *Meta) Position() token.Position {
	return analysisutil.GetPosition(m.Package, m.Pos)
}

func (m *Meta) Compare(other *Meta) int {
	if m.Package.Pkg.Path() != other.Package.Pkg.Path() {
		return strings.Compare(m.Package.Pkg.Path(), other.Package.Pkg.Path())
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

func (m *Meta) LogAttr() slog.Attr {
	return slog.Group("ctx", slog.String("package", m.Package.Pkg.Path()), slog.String("file", analysisutil.FLC(m.Position())), slog.String("function", m.Func.Name()))
}
