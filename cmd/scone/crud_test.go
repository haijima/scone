package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func Test_runCrud_singleProject(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	v := viper.New()
	v.Set("dir", "./testdata/src/isucon13")
	//v.Set("dir", "../../../../risuwork/risuwork/webapp/go")
	v.Set("pattern", "./...")
	//v.Set("format", "table")
	//v.Set("filter", "hash == '2ecb4a0c'")
	v.Set("v", 1)
	//v.Set("analyze-funcs", []string{"github.com/isucon/isucon12-qualify/webapp/go.GetContext@2", "github.com/isucon/isucon12-qualify/webapp/go.SelectContext@2", "github.com/isucon/isucon12-qualify/webapp/go.ExecContext@1"})

	err := runCrud(cmd, v)
	assert.NoError(t, err)
}
