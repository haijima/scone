package query

import (
	"crypto/sha1"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/fatih/color"
	"github.com/haijima/scone/internal/tablecheck"
	"github.com/haijima/scone/internal/tablecheck/query"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "query"
	cmd.Short = "List SQL queries"
	cmd.RunE = func(cmd *cobra.Command, args []string) error { return run(cmd, v) }

	cmd.Flags().StringP("dir", "d", ".", "The directory to analyze")
	cmd.Flags().StringP("pattern", "p", "./...", "The pattern to analyze")
	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|simple}")
	cmd.Flags().StringArray("sort", []string{"file"}, "The sort `keys` {file|type|table}")
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

	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	format := v.GetString("format")
	//sortKeys := v.GetStringSlice("sort")
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

	opt := &query.QueryOption{
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
	}
	result, err := tablecheck.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}

	queries := make([]*query.Query, 0)
	for _, res := range result {
		queries = append(queries, res.QueryResult.Queries...)
	}
	if format == "table" {
		printTable(cmd.OutOrStdout(), queries)
	} else if format == "md" {
		printMarkdown(cmd.OutOrStdout(), queries)
	} else if format == "simple" {
		printSimple(cmd.OutOrStdout(), queries)
	} else if format == "csv" {
		return printCSV(cmd.OutOrStdout(), queries, false)
	} else if format == "tsv" {
		return printCSV(cmd.OutOrStdout(), queries, true)
	} else {
		return fmt.Errorf("unknown format: %s", format)
	}
	return nil
}

func printTable(w io.Writer, queries []*query.Query) {
	table := tablewriter.NewWriter(w)
	table.SetColWidth(tablewriter.MAX_ROW_WIDTH * 4)
	printWithTableWriter(table, queries)
}

func printMarkdown(w io.Writer, queries []*query.Query) {
	table := tablewriter.NewWriter(w)
	table.SetAutoWrapText(false)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	printWithTableWriter(table, queries)
}

func printSimple(w io.Writer, queries []*query.Query) {
	table := tablewriter.NewWriter(w)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	//table.SetTablePadding("\t") // pad with tabs
	table.SetTablePadding(" ")
	table.SetNoWhiteSpace(true)
	printWithTableWriter(table, queries)
}

func printWithTableWriter(w *tablewriter.Table, queries []*query.Query) {
	w.SetHeader([]string{"Package", "Package Path", "File", "Function", "Type", "Table", "Tables", "Sha1", "Query"})
	for _, q := range queries {
		w.Append(row(q))
	}
	w.Render()
}

func printCSV(w io.Writer, queries []*query.Query, isTSV bool) error {
	writer := csv.NewWriter(w)
	if isTSV {
		writer.Comma = '\t'
	}
	err := writer.Write([]string{"Package", "Package Path", "File", "Function", "Type", "Table", "Tables", "Sha1", "Query"})
	if err != nil {
		return err
	}
	for _, q := range queries {
		if err := writer.Write(row(q)); err != nil {
			return err
		}
	}
	writer.Flush()
	return nil
}

var joinPattern = regexp.MustCompile("(?i)(JOIN `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?(?:(?: as)? [a-z0-9_]+)? (?:ON|USING)?)")
var subqueryPattern = regexp.MustCompile("(?i)(SELECT .+? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var insertPattern = regexp.MustCompile("^(?i)(INSERT(?: IGNORE)?(?: INTO)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")
var updatePattern = regexp.MustCompile("^(?i)(UPDATE(?: IGNORE)? `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`? SET)")
var deletePattern = regexp.MustCompile("^(?i)(DELETE(?: IGNORE)? FROM `?(?:[a-z0-9_]+\\.)?)([a-z0-9_]+)(`?)")

func row(q *query.Query) []string {
	file := fmt.Sprintf("%s:%d:%d", filepath.Base(q.Pos.Filename), q.Pos.Line, q.Pos.Column)
	sqlType := q.Kind.String()
	switch q.Kind {
	case query.Select:
		sqlType = color.BlueString(sqlType)
	case query.Insert:
		sqlType = color.GreenString(sqlType)
	case query.Update:
		sqlType = color.YellowString(sqlType)
	case query.Delete:
		sqlType = color.RedString(sqlType)
	}

	emphasize := color.New(color.Bold, color.Underline).SprintFunc()
	raw := q.Raw
	raw = subqueryPattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = joinPattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = insertPattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = updatePattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = deletePattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")

	h := sha1.New()
	h.Write([]byte(q.Raw))

	return []string{
		q.Package.Name(),
		q.Package.Path(),
		file,
		q.Func.Name(),
		sqlType,
		q.Tables[0],
		strconv.Itoa(len(q.Tables)),
		fmt.Sprintf("%x", h.Sum(nil))[:8],
		raw,
	}
}
