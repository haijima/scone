package query

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_parse(t *testing.T) {
	query := `SELECT * FROM t1;`

	got, err := parse(query)

	assert.NotNil(t, got)
	assert.NoError(t, err)
}
