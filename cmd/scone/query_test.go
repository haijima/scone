package main

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/sebdah/goldie/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func Test_runQuery(t *testing.T) {
	tests := []string{"isucon10-qualify", "isucon10-final", "isucon11-qualify", "isucon11-final", "isucon12-qualify", "isucon12-final", "isucon13"}
	for _, tt := range tests {
		t.Run(tt, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			buf := &bytes.Buffer{}
			cmd.SetOut(buf)
			cmd.SetErr(io.Discard)
			v := viper.New()
			v.Set("dir", "./testdata/src/"+tt)
			v.Set("pattern", "./...")
			v.Set("format", "table")
			v.Set("mode", "ssa-method")
			v.Set("analyze-funcs", []string{"github.com/isucon/isucon12-qualify/webapp/go#GetContext#2", "github.com/isucon/isucon12-qualify/webapp/go#SelectContext#2", "github.com/isucon/isucon12-qualify/webapp/go#ExecContext#1"})

			err := runQuery(cmd, v)
			assert.NoError(t, err)

			g := goldie.New(t)
			g.Assert(t, tt, buf.Bytes())
		})
	}
}