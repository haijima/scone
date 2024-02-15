package sql

import (
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

func TestQueryGroups_AllTableMap(t *testing.T) {
	q1 := &Query{Raw: "SELECT * FROM t1 where t1.id = ?", Kind: Select, MainTable: "t1", Tables: []string{"t1"}, FilterColumnMap: map[string]mapset.Set[string]{"t1": mapset.NewSet("id")}}
	q2 := &Query{Raw: "SELECT * FROM t2 where t2.id = ?", Kind: Select, MainTable: "t2", Tables: []string{"t2"}, FilterColumnMap: map[string]mapset.Set[string]{"t2": mapset.NewSet("id")}}
	q3 := &Query{Raw: "SELECT * FROM t3 where t3.id = ?", Kind: Select, MainTable: "t3", Tables: []string{"t3"}, FilterColumnMap: map[string]mapset.Set[string]{"t3": mapset.NewSet("id")}}
	q4 := &Query{Raw: "SELECT * FROM t1 where t1.name = ?", Kind: Select, MainTable: "t1", Tables: []string{"t1"}, FilterColumnMap: map[string]mapset.Set[string]{"t1": mapset.NewSet("name")}}
	q5 := &Query{Raw: "INSERT INTO t1(id, name) SELECT id, name FROM t2 where t2.id = ? AND t2.name = ?", Kind: Insert, MainTable: "t1", Tables: []string{"t1", "t2"}, FilterColumnMap: map[string]mapset.Set[string]{"t2": mapset.NewSet("id", "name")}}
	q6 := &Query{Raw: "SELECT * FROM t1 where t1.id = ? AND t1.name = ?", Kind: Select, MainTable: "t1", Tables: []string{"t1"}, FilterColumnMap: map[string]mapset.Set[string]{"t1": mapset.NewSet("id", "name")}}
	qg1 := NewQueryGroupFrom(q1, q2, q3)
	qg2 := NewQueryGroupFrom(q4, q5, q6)
	qgs := QueryGroups{qg1, qg2}

	tables := qgs.AllTableMap()

	assert.Equalf(t, 3, len(tables), "len(tables)")
	assert.Equalf(t, "t1", tables["t1"].Name, "tables[t1].Name")
	assert.Equalf(t, []QueryKind{Select, Insert}, tables["t1"].Kinds(), "tables[t1].Kinds()")
	assert.Equalf(t, []string{}, tables["t1"].PartitionKeys(), "tables[t1].PartitionKeys()")
	assert.Equalf(t, Insert, tables["t1"].MaxKind(), "tables[t1].MaxKind()")
	assert.Equalf(t, ReadThrough, tables["t1"].Cacheability(), "tables[t1].Cacheability()")
	assert.Equalf(t, "t2", tables["t2"].Name, "tables[t2].Name")
	assert.Equalf(t, []QueryKind{Select, Insert}, tables["t2"].Kinds(), "tables[t2].Kinds()") // FIXME: should be just Select
	assert.Equalf(t, []string{"id"}, tables["t2"].PartitionKeys(), "tables[t2].PartitionKeys()")
	assert.Equalf(t, Insert, tables["t2"].MaxKind(), "tables[t2].MaxKind()")
	assert.Equalf(t, ReadThrough, tables["t2"].Cacheability(), "tables[t2].Cacheability()")
	assert.Equalf(t, "t3", tables["t3"].Name, "tables[t3].Name")
	assert.Equalf(t, []QueryKind{Select}, tables["t3"].Kinds(), "tables[t3].Kinds()")
	assert.Equalf(t, []string{"id"}, tables["t3"].PartitionKeys(), "tables[t3].PartitionKeys()")
	assert.Equalf(t, Select, tables["t3"].MaxKind(), "tables[t3].MaxKind()")
	assert.Equalf(t, HardCoded, tables["t3"].Cacheability(), "tables[t3].Cacheability()")
}

func TestNewQueryGroupFrom(t *testing.T) {
	q1 := &Query{Raw: "SELECT * FROM t1 where t1.id = ?", Kind: Select, MainTable: "t1", Tables: []string{"t1"}, FilterColumnMap: map[string]mapset.Set[string]{"t1": mapset.NewSet("id")}}
	q2 := &Query{Raw: "SELECT * FROM t2 where t2.id = ?", Kind: Select, MainTable: "t2", Tables: []string{"t2"}, FilterColumnMap: map[string]mapset.Set[string]{"t2": mapset.NewSet("id")}}
	qg := NewQueryGroupFrom(q1, q2)
	assert.Equalf(t, 2, qg.List.Cardinality(), "List.Cardinality()")
}

func TestQueryGroup_Queries(t *testing.T) {
	q1 := &Query{Raw: "SELECT * FROM t1 where t1.id = ?", Kind: Select, MainTable: "t1", Tables: []string{"t1"}, FilterColumnMap: map[string]mapset.Set[string]{"t1": mapset.NewSet("id")}}
	q2 := &Query{Raw: "SELECT * FROM t2 where t2.id = ?", Kind: Select, MainTable: "t2", Tables: []string{"t2"}, FilterColumnMap: map[string]mapset.Set[string]{"t2": mapset.NewSet("id")}}
	qg := NewQueryGroupFrom(q1, q2)
	assert.Equalf(t, []*Query{q1, q2}, qg.Queries(), "Queries()")
}

func TestQuery_Hash(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", "da39a3ee"},
		{"normal", "SELECT * FROM t1 where t1.id = ?", "7a525f33"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{Raw: tt.raw}
			assert.Equalf(t, tt.want, q.Hash(), "Hash()")
		})
	}
}

func TestQuery_String(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", ""},
		{"normal", "SELECT * FROM t1 where t1.id = ?", "SELECT * FROM t1 where t1.id = ?"},
		{"long", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id WHERE t1.id = ? AND t2.id = ?", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id WHERE t1.id = ..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &Query{Raw: tt.raw}
			assert.Equalf(t, tt.want, q.String(), "String()")
		})
	}
}

func TestQueryKind(t *testing.T) {
	tests := []struct {
		name               string
		k                  QueryKind
		wantString         string
		wantCRUD           string
		wantColorAttribute color.Attribute
	}{
		{"select", Select, "SELECT", "R", color.FgBlue},
		{"insert", Insert, "INSERT", "C", color.FgGreen},
		{"delete", Delete, "DELETE", "D", color.FgRed},
		{"replace", Replace, "REPLACE", "U", color.FgYellow},
		{"update", Update, "UPDATE", "U", color.FgYellow},
		{"unknown", Unknown, "UNKNOWN", "?", color.FgHiBlack},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.wantString, tt.k.String(), "String()")
			assert.Equalf(t, tt.wantCRUD, tt.k.CRUD(), "CRUD()")
			assert.Equalf(t, tt.wantColorAttribute, tt.k.ColorAttribute(), "ColorAttribute()")
		})
	}
}

func TestQueryKind_ColoredString(t *testing.T) {
	old := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = old }()

	tests := []struct {
		name string
		k    QueryKind
		want string
	}{
		{"select", Select, color.BlueString("SELECT")},
		{"insert", Insert, color.GreenString("INSERT")},
		{"delete", Delete, color.RedString("DELETE")},
		{"replace", Replace, color.YellowString("REPLACE")},
		{"update", Update, color.YellowString("UPDATE")},
		{"unknown", Unknown, color.HiBlackString("UNKNOWN")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.k.ColoredString(), "ColoredString()")
		})
	}
}

func TestParseString(t *testing.T) {
	tests := []struct {
		name      string
		str       string
		wantQuery *Query
		wantOk    bool
	}{
		{"empty", "", nil, false},
		{"SQL", "SELECT * FROM t1 where t1.id = ?", &Query{Kind: Select, Raw: "SELECT * FROM t1 where t1.id = ?", MainTable: "t1", Tables: []string{"t1"}, FilterColumnMap: map[string]mapset.Set[string]{"t1": mapset.NewSet("id")}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotQuery, gotOk := ParseString(tt.str)
			assert.Equalf(t, tt.wantQuery, gotQuery, "ParseString(%v)", tt.str)
			assert.Equalf(t, tt.wantOk, gotOk, "ParseString(%v)", tt.str)
		})
	}
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		str  string
		want string
	}{
		{"empty", "", ""},
		{"whitespace", " ", ""},
		{"whitespace and newline and tab and space", " \n\t ", ""},
		{"duplicate whitespace", "A   B", "A B"},
		{"trailing comment", "A -- B\n C", "A C"},
		{"named parameters", "A = :B", "A = ?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.str)
			assert.Equalf(t, tt.want, got, "Normalize(%v)", tt.str)
		})
	}
}
