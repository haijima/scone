package query

import (
	"go/token"

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
	p := token.NoPos
	for _, pos := range q.Pos {
		if pos.IsValid() {
			p = pos
			break
		}
	}
	return q.Package.Prog.Fset.Position(p)
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
