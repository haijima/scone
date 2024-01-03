package main

import (
	"github.com/haijima/scone/internal/tablecheck"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCallGraphCmd(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "callgraph"
	cmd.Short = "Generate a call graph"
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		dir := v.GetString("dir")
		pattern := v.GetString("pattern")
		return run(dir, pattern)
	}

	cmd.Flags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.Flags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.Flags().String("format", "dot", "The output format {dot|mermaid|text}")
	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(dir, pattern string) error {
	ssa, queryResult, err := tablecheck.Analyze(dir, pattern)
	if err != nil {
		return err
	}

	_, err = tablecheck.CallGraph(ssa, queryResult)
	return err
}
