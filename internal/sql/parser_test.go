package sql

import (
	"testing"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/stretchr/testify/assert"
)

func Test_parse(t *testing.T) {
	query := `
SELECT t1.foo, a2.bar
FROM t1
	JOIN t2 a2 ON t1.foo = t2.foo
	JOIN (SELECT * FROM t3 WHERE t3.id = ?) a3 ON a2.bar = a3.bar
WHERE
    	t1.foo = 'foo'
	AND a2.bar = 'bar'
	AND a2.baz = 'baz'
`

	got, err := parse(query)

	assert.NotNil(t, got)
	assert.NoError(t, err)
}

func Test_parse_kinds(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want QueryKind
	}{
		{"select", "SELECT * FROM t1 where t1.id = ?", Select},
		{"insert", "INSERT INTO t1 (id, name) VALUES (?, ?)", Insert},
		{"update", "UPDATE t1 SET name = ? WHERE id = ?", Update},
		{"replace", "REPLACE INTO t1 (id, name) VALUES (?, ?)", Replace},
		{"delete", "DELETE FROM t1 WHERE id = ?", Delete},
		{"union", "SELECT * FROM t1 UNION SELECT * FROM t2", Select},
		{"ddl", "CREATE TABLE t1 (id INT)", Unknown},
		{"2 SQLs", "SELECT * FROM t1 where t1.id = ?; INSERT INTO t1 (id, name) VALUES (?, ?)", Select},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(tt.sql)
			assert.NotNil(t, got)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got.Kind)
		})
	}
}

func Test_parse_error(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"empty", ""},
		{"invalid", "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parse(tt.sql)
			assert.Error(t, err)
		})
	}
}

func Test_parseSelectStmt(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want map[string]mapset.Set[string]
	}{
		{"no where", "SELECT * FROM t1", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]()}},
		{"simple", "SELECT * FROM t1 where t1.id = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet("id")}},
		{"two where condition", "SELECT * FROM t1 where t1.id = ? AND t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet("id", "name")}},
		{"alias", "SELECT * FROM t1 AS a where a.id = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet("id")}},
		{"join", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id where t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet("name"), "t2": mapset.NewSet[string]()}},
		{"join same table", "SELECT * FROM follow JOIN user AS follower ON follow.follower_id = follower.id JOIN user AS followee ON follow.followee_id = followee.id where followee.name = ?", map[string]mapset.Set[string]{"follow": mapset.NewSet[string](), "user": mapset.NewSet[string]()}},
		{"join subquery", "SELECT * FROM t1 JOIN (SELECT * FROM t2 WHERE t2.id = ?) a2 ON t1.id = a2.id where t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]("name"), "t2": mapset.NewSet[string]("id")}},
		{"join subquery with alias", "SELECT * FROM t1 JOIN (SELECT * FROM t2 WHERE t2.id = ?) AS a2 ON t1.id = a2.id where t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]("name"), "t2": mapset.NewSet[string]("id")}},
		{"join subquery with same table", "SELECT * FROM t1 JOIN (SELECT * FROM t1 WHERE t1.id = ?) a2 ON t1.id = a2.id where t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]()}},
		{"join union", "SELECT * FROM t1 JOIN (SELECT * FROM t2 UNION SELECT * FROM t3) a2 ON t1.id = a2.id where t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]("name"), "t2": mapset.NewSet[string](), "t3": mapset.NewSet[string]()}},
		{"OR condition", "SELECT * FROM t1 where t1.id = ? OR t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]()}},
		{"column without table name", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id where name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]("name"), "t2": mapset.NewSet[string]("name")}},
		{"without table", "SELECT COUNT(*) FROM (SELECT ? AS text) AS texts INNER JOIN (SELECT CONCAT('%', ?, '%') AS pattern) AS patterns ON texts.text LIKE patterns.pattern", map[string]mapset.Set[string]{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.New()
			stmtNodes, warns, err := p.ParseSQL(tt.sql)
			assert.NoError(t, err)
			assert.Empty(t, warns)
			assert.Len(t, stmtNodes, 1)

			stmt := stmtNodes[0]
			assert.IsType(t, stmt, &ast.SelectStmt{})

			parsed := parseStmt(stmt.(*ast.SelectStmt).From, stmt.(*ast.SelectStmt).Where, "")
			assert.Equal(t, tt.want, parsed)
		})
	}
}

func Test_parseSetOprStmt(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want map[string]mapset.Set[string]
	}{
		{"union", "SELECT * FROM t1 UNION SELECT * FROM t2", map[string]mapset.Set[string]{"t1": mapset.NewSet[string](), "t2": mapset.NewSet[string]()}},
		{"union all", "SELECT * FROM t1 UNION ALL SELECT * FROM t2", map[string]mapset.Set[string]{"t1": mapset.NewSet[string](), "t2": mapset.NewSet[string]()}},
		{"union union", "SELECT * FROM t1 UNION (SELECT * FROM t2 UNION SELECT * FROM t3)", map[string]mapset.Set[string]{"t1": mapset.NewSet[string](), "t2": mapset.NewSet[string](), "t3": mapset.NewSet[string]()}},
		{"union with where", "SELECT * FROM t1 UNION SELECT * FROM t2 WHERE t2.id = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string](), "t2": mapset.NewSet("id")}},
		{"union same table", "SELECT * FROM t1 WHERE t1.id = ? UNION SELECT * FROM t1 WHERE t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.New()
			stmtNodes, warns, err := p.ParseSQL(tt.sql)
			assert.NoError(t, err)
			assert.Empty(t, warns)
			assert.Len(t, stmtNodes, 1)

			stmt := stmtNodes[0]
			assert.IsType(t, stmt, &ast.SetOprStmt{})

			parsed := parseSetOprStmt(stmt.(*ast.SetOprStmt), "")
			assert.Equal(t, tt.want, parsed)
		})
	}
}

func Test_parseInsertStmt(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want map[string]mapset.Set[string]
	}{
		{"no where", "INSERT INTO t1 (id, name) VALUES (?, ?)", map[string]mapset.Set[string]{}},
		{"simple", "INSERT INTO t1 (id, name) VALUES (?, ?)", map[string]mapset.Set[string]{}},
		{"column without table name", "INSERT INTO t1 (id, name) VALUES (?, ?)", map[string]mapset.Set[string]{}},
		{"with select", "INSERT INTO t1 (id, name) SELECT id, name FROM t2", map[string]mapset.Set[string]{"t2": mapset.NewSet[string]()}},
		{"with select and where", "INSERT INTO t1 (id, name) SELECT id, name FROM t2 WHERE id = ?", map[string]mapset.Set[string]{"t2": mapset.NewSet("id")}},
		{"with select and join", "INSERT INTO t1 (id, name) SELECT id, name FROM t2 JOIN t3 ON t2.id = t3.id", map[string]mapset.Set[string]{"t2": mapset.NewSet[string](), "t3": mapset.NewSet[string]()}},
		{"with select and join and where", "INSERT INTO t1 (id, name) SELECT id, name FROM t2 JOIN t3 ON t2.id = t3.id WHERE t2.id = ?", map[string]mapset.Set[string]{"t2": mapset.NewSet("id"), "t3": mapset.NewSet[string]()}},
		// FIXME: notify relation between t2.id and t3.id
		{"with select and join and where and subquery", "INSERT INTO t1 (id, name) SELECT id, name FROM t2 JOIN (SELECT * FROM t3 WHERE t3.id = ?) a3 ON t2.id = a3.id WHERE t2.id = ?", map[string]mapset.Set[string]{"t2": mapset.NewSet("id"), "t3": mapset.NewSet("id")}},
		{"with union", "INSERT INTO t1 (id, name) SELECT id, name FROM t2 UNION SELECT id, name FROM t3", map[string]mapset.Set[string]{"t2": mapset.NewSet[string](), "t3": mapset.NewSet[string]()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parser.New()
			stmtNodes, warns, err := p.ParseSQL(tt.sql)
			assert.NoError(t, err)
			assert.Empty(t, warns)
			assert.Len(t, stmtNodes, 1)

			stmt := stmtNodes[0]
			assert.IsType(t, stmt, &ast.InsertStmt{})

			parsed := parseInsertStmt(stmt.(*ast.InsertStmt), "")
			assert.Equal(t, tt.want, parsed)
		})
	}
}
