package sql

import (
	"slices"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
)

type Table struct {
	Name          string
	kinds         mapset.Set[QueryKind]
	filterColumns mapset.Set[string]
}

func NewTable(name string) *Table {
	return &Table{Name: name, filterColumns: mapset.NewSet[string](), kinds: mapset.NewSet[QueryKind]()}
}

func (t *Table) String() string {
	return t.Name
}

func (t *Table) Kinds() []QueryKind {
	return mapset.Sorted(t.kinds)
}

func (t *Table) MaxKind() QueryKind {
	return slices.Max(t.kinds.ToSlice())
}

func (t *Table) PartitionKeys() []string {
	return mapset.Sorted(t.filterColumns)
}

func (t *Table) Cacheability() Cacheability {
	switch t.MaxKind() {
	case Select:
		return Static
	case Insert:
		return Immutable
	case Delete, Replace, Update:
		return Mutable
	default:
		return UnknownCacheability
	}
}

type Cacheability int

const (
	Static Cacheability = iota
	Immutable
	Mutable
	UnknownCacheability
)

func (c Cacheability) String() string {
	switch c {
	case Static:
		return "Static"
	case Immutable:
		return "Immutable"
	case Mutable:
		return "Mutable"
	default:
		return "Unknown"
	}
}

func (c Cacheability) ColoredString() string {
	return c.Color(c.String())
}

func (c Cacheability) Color(str string) string {
	switch c {
	case Static:
		return color.BlueString(str)
	case Immutable:
		return color.GreenString(str)
	case Mutable:
		return color.RedString(str)
	default:
		return color.HiBlackString(str)
	}
}
