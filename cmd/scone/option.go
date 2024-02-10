package main

import (
	"github.com/haijima/scone/internal/analysis"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func SetQueryOptionFlags(cmd *cobra.Command) {
	cmd.Flags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.Flags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.Flags().StringSlice("exclude-queries", []string{}, "The `SHA1s` of queries to exclude")
	cmd.Flags().StringSlice("exclude-packages", []string{}, "The `names` of packages to exclude")
	cmd.Flags().StringSlice("exclude-package-paths", []string{}, "The `paths` of packages to exclude")
	cmd.Flags().StringSlice("exclude-files", []string{}, "The `names` of files to exclude")
	cmd.Flags().StringSlice("exclude-functions", []string{}, "The `names` of functions to exclude")
	cmd.Flags().StringSlice("exclude-query-types", []string{}, "The `types` of queries to exclude {select|insert|update|delete}")
	cmd.Flags().StringSlice("exclude-tables", []string{}, "The `names` of tables to exclude")
	cmd.Flags().StringSlice("filter-queries", []string{}, "The `SHA1s` of queries to filter")
	cmd.Flags().StringSlice("filter-packages", []string{}, "The `names` of packages to filter")
	cmd.Flags().StringSlice("filter-package-paths", []string{}, "The `paths` of packages to filter")
	cmd.Flags().StringSlice("filter-files", []string{}, "The `names` of files to filter")
	cmd.Flags().StringSlice("filter-functions", []string{}, "The `names` of functions to filter")
	cmd.Flags().StringSlice("filter-query-types", []string{}, "The `types` of queries to filter {select|insert|update|delete}")
	cmd.Flags().StringSlice("filter-tables", []string{}, "The `names` of tables to filter")
	cmd.Flags().StringSlice("analyze-funcs", []string{}, "The names of functions to analyze additionally. format: `<package>#<function>#<argument index>`")
	_ = cmd.MarkFlagDirname("dir")
}

func QueryOptionFromViper(v *viper.Viper) *analysis.Option {
	excludeQueries := v.GetStringSlice("exclude-queries")
	excludePackages := v.GetStringSlice("exclude-packages")
	excludePackagePaths := v.GetStringSlice("exclude-package-paths")
	excludeFiles := v.GetStringSlice("exclude-files")
	excludeFunctions := v.GetStringSlice("exclude-functions")
	excludeQueryTypes := v.GetStringSlice("exclude-query-types")
	excludeTables := v.GetStringSlice("exclude-tables")
	filterQueries := v.GetStringSlice("filter-queries")
	filterPackages := v.GetStringSlice("filter-packages")
	filterPackagePaths := v.GetStringSlice("filter-package-paths")
	filterFiles := v.GetStringSlice("filter-files")
	filterFunctions := v.GetStringSlice("filter-functions")
	filterQueryTypes := v.GetStringSlice("filter-query-types")
	filterTables := v.GetStringSlice("filter-tables")
	additionalFuncs := v.GetStringSlice("analyze-funcs")

	return &analysis.Option{
		ExcludeQueries:      excludeQueries,
		ExcludePackages:     excludePackages,
		ExcludePackagePaths: excludePackagePaths,
		ExcludeFiles:        excludeFiles,
		ExcludeFunctions:    excludeFunctions,
		ExcludeQueryTypes:   excludeQueryTypes,
		ExcludeTables:       excludeTables,
		FilterQueries:       filterQueries,
		FilterPackages:      filterPackages,
		FilterPackagePaths:  filterPackagePaths,
		FilterFiles:         filterFiles,
		FilterFunctions:     filterFunctions,
		FilterQueryTypes:    filterQueryTypes,
		FilterTables:        filterTables,
		AdditionalFuncs:     additionalFuncs,
	}
}
