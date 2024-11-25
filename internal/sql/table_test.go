package sql

import (
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

func TestNewTable(t *testing.T) {
	name := "t1"
	got := NewTable(name)

	assert.Equal(t, name, got.Name)
	assert.Equal(t, mapset.NewSet[string](), got.filterColumns)
	assert.Equal(t, mapset.NewSet[QueryKind](), got.kinds)
}

func TestTable_String(t *testing.T) {
	name := "t1"
	table := NewTable(name)

	got := table.String()

	assert.Equal(t, name, got, "String()")
}

func TestTable_PartitionKeys(t1 *testing.T) {
	table := NewTable("t1")
	table.filterColumns.Append("id", "name", "age")

	got := table.PartitionKeys()

	assert.Equal(t1, []string{"age", "id", "name"}, got, "PartitionKeys()")
}

func TestTable_Kinds(t1 *testing.T) {
	table := NewTable("t1")
	table.kinds.Append(Insert, Update, Select)

	got := table.Kinds()

	assert.Equal(t1, []QueryKind{Select, Insert, Update}, got, "Kinds()")
}

func TestTable_MaxKind(t1 *testing.T) {
	table := NewTable("t1")
	table.kinds.Append(Insert, Update, Select, Delete)

	got := table.MaxKind()

	assert.Equal(t1, Update, got, "MaxKind()")
}

func TestTable_Cacheability(t1 *testing.T) {
	tests := []struct {
		name  string
		kinds []QueryKind
		want  Cacheability
	}{
		{"select", []QueryKind{Select}, Static},
		{"insert", []QueryKind{Insert}, Immutable},
		{"update", []QueryKind{Update}, Mutable},
		{"delete", []QueryKind{Delete}, Mutable},
		{"replace", []QueryKind{Replace}, Mutable},
		{"unknown", []QueryKind{Unknown}, UnknownCacheability},
		{"multiple", []QueryKind{Select, Insert}, Immutable},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			table := NewTable("t1")
			table.kinds.Append(tt.kinds...)

			got := table.Cacheability()

			assert.Equalf(t1, tt.want, got, "Cacheability()")
		})
	}
}

func TestCacheability_String(t *testing.T) {
	tests := []struct {
		name string
		c    Cacheability
		want string
	}{
		{"static", Static, "Static"},
		{"immutable", Immutable, "Immutable"},
		{"mutable", Mutable, "Mutable"},
		{"unknown", UnknownCacheability, "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.c.String(), "String()")
		})
	}
}

func TestCacheability_ColoredString(t *testing.T) {
	old := color.NoColor
	color.NoColor = false
	defer func() { color.NoColor = old }()

	tests := []struct {
		name string
		c    Cacheability
		want string
	}{
		{"static", Static, color.BlueString("Static")},
		{"immutable", Immutable, color.GreenString("Immutable")},
		{"mutable", Mutable, color.RedString("Mutable")},
		{"unknown", UnknownCacheability, color.HiBlackString("Unknown")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, tt.c.ColoredString(), "ColoredString()")
		})
	}
}
