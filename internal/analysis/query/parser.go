package query

import (
	"log/slog"

	"github.com/haijima/scone/internal/util"
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
		q.FilterColumnMap = parseStmt(s.From, s.Where)
	case *ast.SetOprStmt:
		q.Kind = Select
		q.FilterColumnMap = parseSetOprStmt(s)
	case *ast.InsertStmt:
		if s.IsReplace {
			q.Kind = Replace
		} else {
			q.Kind = Insert
		}
		q.FilterColumnMap = parseInsertStmt(s)
	case *ast.UpdateStmt:
		q.Kind = Update
		q.FilterColumnMap = parseStmt(s.TableRefs, s.Where)
	case *ast.DeleteStmt:
		q.Kind = Delete
		q.FilterColumnMap = parseStmt(s.TableRefs, s.Where)
	default:
		q.Kind = Unknown
		q.FilterColumnMap = make(map[string][]string)
	}
	return q, nil
}

func parseSetOprStmt(stmt *ast.SetOprStmt) map[string][]string {
	res := make(map[string][]string)
	for _, s := range stmt.SelectList.Selects {
		if stmt, ok := s.(*ast.SelectStmt); ok {
			m := parseStmt(stmt.From, stmt.Where)
			for k, v := range m {
				res[k] = util.Intersect(res[k], v)
			}
		} else if stmt, ok := s.(*ast.SetOprStmt); ok {
			m := parseSetOprStmt(stmt)
			for k, v := range m {
				res[k] = util.Intersect(res[k], v)
			}
		}
	}
	return res
}

func parseInsertStmt(stmt *ast.InsertStmt) map[string][]string {
	if stmt.Select != nil {
		switch s := stmt.Select.(type) {
		case *ast.SelectStmt:
			return parseStmt(s.From, s.Where)
		case *ast.SetOprStmt:
			return parseSetOprStmt(s)
		case *ast.SubqueryExpr:
			return parseStmt(s.Query.(*ast.SelectStmt).From, s.Query.(*ast.SelectStmt).Where)
		}
	}
	return make(map[string][]string)
}

func parseStmt(tableRefs *ast.TableRefsClause, condition ast.Node) map[string][]string {
	if tableRefs == nil || tableRefs.TableRefs == nil {
		return make(map[string][]string)
	}

	jf := &JoinFlatter{tableNames: make([]*ast.TableSource, 0), selectStmts: make([]*ast.TableSource, 0), setOprStmts: make([]*ast.TableSource, 0)}
	tableRefs.Accept(jf)

	// Collect table names and aliases
	tableAliases := make(map[string]string) // key: alias, value: original table name
	for _, t := range jf.tableNames {
		name := t.Source.(*ast.TableName).Name.L
		asName := t.AsName.L
		if asName == "" {
			asName = name
		}
		tableAliases[asName] = name
	}

	// Collect column names
	ra := make(map[string][]string)
	for a := range tableAliases {
		ra[a] = make([]string, 0)
	}
	if condition != nil {
		vc := &colX{}
		condition.Accept(vc)
		for _, col := range vc.colNames {
			tableAlias := col.Table.L
			if tableAlias == "" {
				if len(jf.tableNames) == 1 && len(jf.selectStmts) == 0 && len(jf.setOprStmts) == 0 {
					tableAlias = jf.tableNames[0].Source.(*ast.TableName).Name.L
				} else {
					slog.Warn("ambiguous column name in (sub)query", "column", col.Name.L, "condition", condition.Text())
					for a := range tableAliases {
						ra[a] = append(ra[a], col.Name.L)
					}
					continue
				}
			}
			ra[tableAlias] = append(ra[tableAlias], col.Name.L)
		}
	}

	// Intersect column names
	res := make(map[string][]string)
	for tableAlias, colNames := range ra {
		if tableName, ok := tableAliases[tableAlias]; ok {
			res[tableName] = util.Intersect(res[tableName], colNames)
		}
	}
	for _, tableName := range jf.tableNames {
		name := tableName.Source.(*ast.TableName).Name.L
		if _, ok := res[name]; !ok {
			res[name] = make([]string, 0)
		}
	}

	// Intersect column names with result of sub-queries
	for _, s := range jf.selectStmts {
		m := parseStmt(s.Source.(*ast.SelectStmt).From, s.Source.(*ast.SelectStmt).Where)
		for k, v := range m {
			res[k] = util.Intersect(res[k], v)
		}
	}
	return res
}
