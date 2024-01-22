package query

import (
	"testing"

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
		want map[string][]string
	}{
		{"no where", "SELECT * FROM t1", map[string][]string{"t1": {}}},
		{"simple", "SELECT * FROM t1 where t1.id = ?", map[string][]string{"t1": {"id"}}},
		{"two where condition", "SELECT * FROM t1 where t1.id = ? AND t1.name = ?", map[string][]string{"t1": {"id", "name"}}},
		{"alias", "SELECT * FROM t1 AS a where a.id = ?", map[string][]string{"t1": {"id"}}},
		{"join", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id where t1.name = ?", map[string][]string{"t1": {"name"}, "t2": {}}},
		{"join same table", "SELECT * FROM follow JOIN user AS follower ON follow.follower_id = follower.id JOIN user AS followee ON follow.followee_id = followee.id where followee.name = ?", map[string][]string{"follow": {}, "user": {}}},
		{"join subquery", "SELECT * FROM t1 JOIN (SELECT * FROM t2 WHERE t2.id = ?) a2 ON t1.id = a2.id where t1.name = ?", map[string][]string{"t1": {"name"}, "t2": {"id"}}},
		{"join subquery with alias", "SELECT * FROM t1 JOIN (SELECT * FROM t2 WHERE t2.id = ?) AS a2 ON t1.id = a2.id where t1.name = ?", map[string][]string{"t1": {"name"}, "t2": {"id"}}},
		{"join subquery with same table", "SELECT * FROM t1 JOIN (SELECT * FROM t1 WHERE t1.id = ?) a2 ON t1.id = a2.id where t1.name = ?", map[string][]string{"t1": {}}},
		{"OR condition", "SELECT * FROM t1 where t1.id = ? OR t1.name = ?", map[string][]string{"t1": {}}},
		{"column without table name", "SELECT * FROM t1 JOIN t2 ON t1.id = t2.id where name = ?", map[string][]string{"t1": {"name"}, "t2": {"name"}}},
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

			parsed := parseSelectStmt(stmt.(*ast.SelectStmt))
			assert.Equal(t, tt.want, parsed)
		})
	}
}
