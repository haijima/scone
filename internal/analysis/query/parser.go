package query

import (
	"log/slog"

	"github.com/lmittmann/tint"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/opcode"
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

type colX struct {
	colNames []*ast.ColumnName
}

func (v *colX) Enter(in ast.Node) (ast.Node, bool) {
	if bin, ok := in.(*ast.BinaryOperationExpr); ok {
		if bin.Op == opcode.EQ {
			if col, ok := bin.L.(*ast.ColumnNameExpr); ok {
				v.colNames = append(v.colNames, col.Name)
			}
			if col, ok := bin.R.(*ast.ColumnNameExpr); ok {
				v.colNames = append(v.colNames, col.Name)
			}
			return in, false
		} else if bin.Op == opcode.LogicAnd {
			return in, false
		} else if bin.Op == opcode.LogicOr {
			return in, true
		} else {
			return in, true
		}
	}
	return in, true
}

func (v *colX) Leave(in ast.Node) (ast.Node, bool) {
	return in, true
}

type JoinFlatter struct {
	tableNames  []*ast.TableSource
	selectStmts []*ast.TableSource
	setOprStmts []*ast.TableSource
}

func (v *JoinFlatter) Enter(in ast.Node) (ast.Node, bool) {
	switch t := in.(type) {
	case *ast.TableSource:
		switch t.Source.(type) {
		case *ast.TableName:
			v.tableNames = append(v.tableNames, t)
			return in, true
		case *ast.SelectStmt:
			v.selectStmts = append(v.selectStmts, t)
			return in, true
		case *ast.SetOprStmt:
			v.setOprStmts = append(v.setOprStmts, t)
			return in, true
		default:
			return in, true
		}
	case *ast.Join:
		return in, false
	default:
		return in, false
	}
}

func (v *JoinFlatter) Leave(in ast.Node) (ast.Node, bool) {
	return in, true
}

func parse(sql string) (*Query, error) {
	p := parser.New()
	stmtNodes, warns, err := p.ParseSQL(sql)
	if err != nil {
		return nil, err
	}
	for _, w := range warns {
		slog.Warn("warning when parsing SQL", "SQL", sql, tint.Err(w))
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
		q.FilterColumnMap = parseSelectStmt(s)
	case *ast.SetOprStmt:
		q.Kind = Select
		q.FilterColumnMap = parseSetOprStmt(s)
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

func parseSetOprStmt(stmt *ast.SetOprStmt) map[string][]string {
	res := make(map[string][]string)
	for _, s := range stmt.SelectList.Selects {
		if stmt, ok := s.(*ast.SelectStmt); ok {
			m := parseSelectStmt(stmt)
			for k, v := range m {
				res[k] = Intersect(res[k], v)
			}
		} else if stmt, ok := s.(*ast.SetOprStmt); ok {
			m := parseSetOprStmt(stmt)
			for k, v := range m {
				res[k] = Intersect(res[k], v)
			}
		}
	}
	return res
}

func parseSelectStmt(stmt *ast.SelectStmt) map[string][]string {
	jf := &JoinFlatter{tableNames: make([]*ast.TableSource, 0), selectStmts: make([]*ast.TableSource, 0), setOprStmts: make([]*ast.TableSource, 0)}
	if stmt.From != nil {
		stmt.From.Accept(jf)
	} else {
		slog.Warn("no from clause in (sub)query", "SQL", stmt.Text())
	}

	tableAliases := make(map[string]string) // key: alias, value: original table name
	for _, t := range jf.tableNames {
		name := t.Source.(*ast.TableName).Name.L
		asName := t.AsName.L
		if asName == "" {
			asName = name
		}
		tableAliases[asName] = name
	}

	ra := make(map[string][]string)
	for a := range tableAliases {
		ra[a] = make([]string, 0)
	}
	if stmt.Where != nil {
		vc := &colX{}
		stmt.Where.Accept(vc)
		for _, col := range vc.colNames {
			tableAlias := col.Table.L
			if tableAlias == "" {
				if len(jf.tableNames) == 1 && len(jf.selectStmts) == 0 && len(jf.setOprStmts) == 0 {
					tableAlias = jf.tableNames[0].Source.(*ast.TableName).Name.L
				} else {
					slog.Warn("ambiguous column name in (sub)query", "column", col.Name.L, "SQL", stmt.Text())
					for a := range tableAliases {
						ra[a] = append(ra[a], col.Name.L)
					}
					continue
				}
			}

			ra[tableAlias] = append(ra[tableAlias], col.Name.L)
		}
	}

	res := make(map[string][]string)
	for tableAlias, colNames := range ra {
		if tableName, ok := tableAliases[tableAlias]; ok {
			res[tableName] = Intersect(res[tableName], colNames)
		}
	}
	for _, tableName := range jf.tableNames {
		name := tableName.Source.(*ast.TableName).Name.L
		if _, ok := res[name]; !ok {
			res[name] = make([]string, 0)
		}
	}

	for _, s := range jf.selectStmts {
		m := parseSelectStmt(s.Source.(*ast.SelectStmt))
		for k, v := range m {
			res[k] = Intersect(res[k], v)
		}
	}
	return res
}

func Intersect(a, b []string) []string {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	seen := make(map[string]bool)
	for _, v := range a {
		seen[v] = true
	}
	r := make([]string, 0)
	for _, v := range b {
		if seen[v] {
			r = append(r, v)
		}
	}
	return r
}
