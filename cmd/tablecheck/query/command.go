package query

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

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
	cmd.Flags().StringSlice("sort", []string{"file"}, "The sort `keys` {file|function|type|table|sha1}")
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
	cmd.Flags().StringSlice("cols", []string{}, "The `columns` to show {"+strings.Join(headerColumns, "|")+"}")
	cmd.Flags().Bool("no-header", false, "Hide header")
	cmd.Flags().Bool("no-rownum", false, "Hide row number")
	cmd.Flags().String("mode", "ssa-method", "The query analyze `mode` {ssa-method|ssa-const|ast}")

	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

var headerColumns = []string{"package", "package-path", "file", "function", "type", "table", "tables", "sha1", "query", "raw-query"}
var defaultHeaderIndex = []int{0, 1, 2, 3, 4, 5, 6, 7, 8}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	format := v.GetString("format")
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
	cols := v.GetStringSlice("cols")
	noHeader := v.GetBool("no-header")
	noRowNum := v.GetBool("no-rownum")
	sortKeys := v.GetStringSlice("sort")
	modeFlg := v.GetString("mode")

	var mode query.AnalyzeMode
	if modeFlg == "ssa-method" {
		mode = query.SsaMethod
	} else if modeFlg == "ssa-const" {
		mode = query.SsaConst
	} else if modeFlg == "ast" {
		mode = query.Ast
	} else {
		return fmt.Errorf("unknown mode: %s", modeFlg)
	}

	opt := &query.Option{
		Mode:                mode,
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

	for _, sortKey := range sortKeys {
		if sortKey != "file" && sortKey != "function" && sortKey != "type" && sortKey != "table" && sortKey != "sha1" {
			return fmt.Errorf("unknown sort key: %s", sortKey)
		}
	}
	if !slices.Contains(sortKeys, "file") {
		sortKeys = append(sortKeys, "file")
	}
	slices.SortFunc(queries, func(a, b *query.Query) int {
		for _, sortKey := range sortKeys {
			if sortKey == "function" && a.Func.Name() != b.Func.Name() {
				return strings.Compare(a.Func.Name(), b.Func.Name())
			} else if sortKey == "type" && a.Kind != b.Kind {
				return int(a.Kind) - int(b.Kind)
			} else if sortKey == "table" && a.Tables[0] != b.Tables[0] {
				return strings.Compare(a.Tables[0], b.Tables[0])
			} else if sortKey == "sha1" && a.Sha() != b.Sha() {
				return strings.Compare(a.Sha(), b.Sha())
			} else if sortKey == "file" {
				if a.Package.Pkg.Path() != b.Package.Pkg.Path() {
					return strings.Compare(a.Package.Pkg.Path(), b.Package.Pkg.Path())
				} else if a.Position().Filename != b.Position().Filename {
					return strings.Compare(a.Position().Filename, b.Position().Filename)
				} else if a.Position().Line != b.Position().Line {
					return a.Position().Line - b.Position().Line
				} else if a.Position().Column != b.Position().Column {
					return a.Position().Column - b.Position().Column
				}
			}
		}
		return 0
	})

	printOpt := &PrintOption{Cols: defaultHeaderIndex, NoHeader: noHeader, NoRowNum: noRowNum}
	if len(cols) > 0 {
		printOpt.Cols = make([]int, 0, len(cols))
		for _, col := range cols {
			if !slices.Contains(headerColumns, col) {
				return fmt.Errorf("unknown columns: %s", col)
			}
			for i, header := range headerColumns {
				if col == header {
					printOpt.Cols = append(printOpt.Cols, i)
					break
				}
			}
		}
	}
	if format == "table" {
		printTable(cmd.OutOrStdout(), queries, printOpt)
	} else if format == "md" {
		printMarkdown(cmd.OutOrStdout(), queries, printOpt)
	} else if format == "simple" {
		printSimple(cmd.OutOrStdout(), queries, printOpt)
	} else if format == "csv" {
		return printCSV(cmd.OutOrStdout(), queries, false, printOpt)
	} else if format == "tsv" {
		return printCSV(cmd.OutOrStdout(), queries, true, printOpt)
	} else {
		return fmt.Errorf("unknown format: %s", format)
	}
	return nil
}

type PrintOption struct {
	Cols     []int
	NoHeader bool
	NoRowNum bool
}

func printTable(w io.Writer, queries []*query.Query, opt *PrintOption) {
	table := tablewriter.NewWriter(w)
	table.SetColWidth(tablewriter.MAX_ROW_WIDTH * 4)
	table.SetAutoWrapText(false)
	printWithTableWriter(table, queries, opt)
}

func printMarkdown(w io.Writer, queries []*query.Query, opt *PrintOption) {
	table := tablewriter.NewWriter(w)
	table.SetAutoWrapText(false)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	printWithTableWriter(table, queries, opt)
}

func printSimple(w io.Writer, queries []*query.Query, opt *PrintOption) {
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
	printWithTableWriter(table, queries, opt)
}

func printWithTableWriter(w *tablewriter.Table, queries []*query.Query, opt *PrintOption) {
	if !opt.NoHeader {
		w.SetHeader(makeHeader(opt))
	}
	for i, q := range queries {
		r := row(q, opt)
		if !opt.NoRowNum {
			r = append([]string{strconv.Itoa(i + 1)}, r...)
		}
		w.Append(r)
	}
	w.Render()
}

func printCSV(w io.Writer, queries []*query.Query, isTSV bool, opt *PrintOption) error {
	writer := csv.NewWriter(w)
	if isTSV {
		writer.Comma = '\t'
	}
	if !opt.NoHeader {
		if err := writer.Write(makeHeader(opt)); err != nil {
			return err
		}
	}
	for i, q := range queries {
		r := row(q, opt)
		if !opt.NoRowNum {
			r = append([]string{strconv.Itoa(i + 1)}, r...)
		}
		if err := writer.Write(r); err != nil {
			return err
		}
	}
	writer.Flush()
	return nil
}

func makeHeader(opt *PrintOption) []string {
	header := make([]string, 0)
	if !opt.NoRowNum {
		header = append(header, "#")
	}
	for _, col := range opt.Cols {
		header = append(header, strings.ReplaceAll(headerColumns[col], "-", " "))
	}
	return header
}

func row(q *query.Query, opt *PrintOption) []string {
	file := fmt.Sprintf("%s:%d:%d", filepath.Base(q.Position().Filename), q.Position().Line, q.Position().Column)
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
	raw = query.SubQueryPattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = query.JoinPattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = query.InsertPattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = query.UpdatePattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")
	raw = query.DeletePattern.ReplaceAllString(raw, "$1"+emphasize("$2")+"$3")

	ellipsis := raw
	if len(ellipsis) > 60 {
		lastSpaceIx := -1
		for i, r := range ellipsis {
			if unicode.IsSpace(r) {
				lastSpaceIx = i
			}
			if i >= 60-4 && lastSpaceIx != -1 {
				ellipsis = ellipsis[:lastSpaceIx] + " ..."
				break
			}
		}
	}

	fullRow := []string{
		q.Package.Pkg.Name(),
		q.Package.Pkg.Path(),
		file,
		q.Func.Name(),
		sqlType,
		q.Tables[0],
		strconv.Itoa(len(q.Tables)),
		q.Sha(),
		ellipsis,
		raw,
	}
	res := make([]string, 0, len(opt.Cols))
	for _, col := range opt.Cols {
		res = append(res, fullRow[col])
	}
	return res
}
