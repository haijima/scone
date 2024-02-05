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
		{"OR condition", "SELECT * FROM t1 where t1.id = ? OR t1.name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]()}},
		{"column without table name", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id where name = ?", map[string]mapset.Set[string]{"t1": mapset.NewSet[string]("name"), "t2": mapset.NewSet[string]("name")}},
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
