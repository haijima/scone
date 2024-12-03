package main

import (
	"bytes"
	"fmt"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewReportCmd(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "report"
	cmd.Aliases = []string{"reports"}
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return RunReport(cmd, v)
	}

	cmd.Flags().String("format", "text", "The output format {text|html}")

	return cmd
}

func RunReport(cmd *cobra.Command, v *viper.Viper) error {

	format := v.GetString("format")
	if format != "text" && format != "html" {
		return fmt.Errorf("invalid format: %s", format)
	} else if format == "text" {
		v.Set("format", "table")
	}

	var buf bytes.Buffer
	cmd.SetOut(&buf)

	if err := runCrud(cmd, v); err != nil {
		return err
	}

	fmt.Println(buf.String())
	buf.Truncate(0)

	if err := runTable(cmd, v); err != nil {
		return err
	}
	fmt.Println(buf.String())
	buf.Truncate(0)

	if err := runQuery(cmd, v); err != nil {
		return err
	}
	fmt.Println(buf.String())
	buf.Truncate(0)

	if err := runLoop(cmd, v); err != nil {
		return err
	}
	fmt.Println(buf.String())
	buf.Truncate(0)

	return nil
}
