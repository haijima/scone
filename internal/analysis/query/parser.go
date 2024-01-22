package query

import (
	"log/slog"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
)

type tableX struct {
	tableNames []string
}

func (v *tableX) Enter(in ast.Node) (ast.Node, bool) {
	if name, ok := in.(*ast.TableName); ok {
		v.tableNames = append(v.tableNames, name.Name.O)
	}
	return in, false
}

func (v *tableX) Leave(in ast.Node) (ast.Node, bool) {
	return in, true
}

func parse(sql string) (*Query, error) {
	p := parser.New()

	stmtNodes, warns, err := p.ParseSQL(sql)
	if err != nil {
		return nil, err
	}
	for _, w := range warns {
		slog.Warn(w.Error())
	}
	if len(stmtNodes) > 1 {
		slog.Warn("multiple statements in one query")
	}

	stmt := stmtNodes[0]
	q := &Query{Raw: stmt.Text()}

	v := &tableX{}
	stmt.Accept(v)
	tables := v.tableNames
	if len(tables) > 0 {
		q.MainTable = tables[0]
	}
	tableSet := make([]string, 0)
	seen := make(map[string]bool)
	for _, t := range tables {
		if !seen[t] {
			tableSet = append(tableSet, t)
			seen[t] = true
		}
	}
	q.Tables = tableSet // q.Tables[0] == q.MainTable

	switch s := stmt.(type) {
	case *ast.SelectStmt:
		q.Kind = Select
	case *ast.SetOprStmt:
		q.Kind = Select
	case *ast.InsertStmt:
		if s.IsReplace {
			q.Kind = Replace
		} else {
			q.Kind = Insert
		}
	case *ast.UpdateStmt:
		q.Kind = Update
	case *ast.DeleteStmt:
		q.Kind = Delete
	default:
		q.Kind = Unknown
	}
	return q, nil
}
