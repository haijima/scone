package analysis

import (
	"slices"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/analysisutil/ssautil"
	"github.com/haijima/scone/internal/sql"
	"golang.org/x/exp/maps"
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
	Posx        *ssautil.Posx
	FromComment bool
}

func NewQueryResult(pos *ssautil.Posx) *QueryResult {
	return &QueryResult{QueryGroup: sql.NewQueryGroup(), Posx: pos}
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
	if !qr.Posx.Equal(other.Posx) {
		return qr.Posx.Compare(other.Posx)
	}
	return slices.CompareFunc(qr.Queries(), other.Queries(), func(a, b *sql.Query) int { return strings.Compare(a.Hash(), b.Hash()) })
}
