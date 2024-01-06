package query

import "golang.org/x/tools/go/ssa"

type Query struct {
	Kind   QueryKind
	Func   *ssa.Function
	Name   string
	Raw    string
	Tables []string
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
