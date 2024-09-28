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

func Test_runLoop_singleProject(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(io.Discard)
	v := viper.New()
	v.Set("dir", "./testdata/src/loop")
	v.Set("pattern", "./...")
	v.Set("format", "table")
	v.Set("v", 1)

	err := runLoop(cmd, v)
	assert.NoError(t, err)
}
