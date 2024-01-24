package main

import (
	"github.com/haijima/cobrax"
	"github.com/haijima/scone/cmd/scone/callgraph"
	"github.com/haijima/scone/cmd/scone/query"
	"github.com/haijima/scone/cmd/scone/table"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewRootCmd(v *viper.Viper, fs afero.Fs) *cobra.Command {
	cmd := cobrax.NewRoot(v)
	cmd.Use = "scone"
	cmd.Short = "scone is a static analysis tool for SQL"
	cmd.Version = cobrax.VersionFunc()
	cmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return cobrax.RootPersistentPreRunE(cmd, v, fs, args)
	}

	cmd.PersistentFlags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.PersistentFlags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.PersistentFlags().StringSlice("exclude-queries", []string{}, "The `SHA1s` of queries to exclude")
	cmd.PersistentFlags().StringSlice("exclude-packages", []string{}, "The `names` of packages to exclude")
	cmd.PersistentFlags().StringSlice("exclude-package-paths", []string{}, "The `paths` of packages to exclude")
	cmd.PersistentFlags().StringSlice("exclude-files", []string{}, "The `names` of files to exclude")
	cmd.PersistentFlags().StringSlice("exclude-functions", []string{}, "The `names` of functions to exclude")
	cmd.PersistentFlags().StringSlice("exclude-query-types", []string{}, "The `types` of queries to exclude {select|insert|update|delete}")
	cmd.PersistentFlags().StringSlice("exclude-tables", []string{}, "The `names` of tables to exclude")
	cmd.PersistentFlags().StringSlice("filter-queries", []string{}, "The `SHA1s` of queries to filter")
	cmd.PersistentFlags().StringSlice("filter-packages", []string{}, "The `names` of packages to filter")
	cmd.PersistentFlags().StringSlice("filter-package-paths", []string{}, "The `paths` of packages to filter")
	cmd.PersistentFlags().StringSlice("filter-files", []string{}, "The `names` of files to filter")
	cmd.PersistentFlags().StringSlice("filter-functions", []string{}, "The `names` of functions to filter")
	cmd.PersistentFlags().StringSlice("filter-query-types", []string{}, "The `types` of queries to filter {select|insert|update|delete}")
	cmd.PersistentFlags().StringSlice("filter-tables", []string{}, "The `names` of tables to filter")
	cmd.PersistentFlags().StringSlice("analyze-funcs", []string{}, "The names of functions to analyze additionally. format: `<package>#<function>#<argument index>`")
	cmd.PersistentFlags().String("mode", "ssa-method", "The query analyze `mode` {ssa-method|ssa-const|ast}")
	_ = cmd.MarkFlagDirname("dir")

	cmd.AddCommand(callgraph.NewCommand(v, fs))
	cmd.AddCommand(query.NewCommand(v, fs))
	cmd.AddCommand(table.NewCommand(v, fs))

	cmd.SetGlobalNormalizationFunc(cobrax.SnakeToKebab)

	return cmd
}
