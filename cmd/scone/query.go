package main

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	internalio "github.com/haijima/scone/internal/io"
	"github.com/haijima/scone/internal/sql"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewQueryCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "query"
	cmd.Short = "List SQL queries"
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return runQuery(cmd, v) }

	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|simple}")
	cmd.Flags().StringSlice("sort", []string{"file"}, "The sort `keys` {file|function|type|table|sha1}")
	cmd.Flags().StringSlice("cols", []string{}, "The `columns` to show {"+strings.Join(headerColumns, "|")+"}")
	cmd.Flags().Bool("no-header", false, "Hide header")
	cmd.Flags().Bool("no-rownum", false, "Hide row number")
	cmd.Flags().Bool("full-package-path", false, "Show full package path")
	cmd.Flags().Bool("expand-query-group", false, "Expand query group")
	SetQueryOptionFlags(cmd)

	return cmd
}

var headerColumns = []string{"package", "package-path", "file", "function", "type", "tables", "sha1", "query", "raw-query"}
var defaultHeaderIndex = []int{0, 1, 2, 3, 4, 5, 6, 7}

func runQuery(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	format := v.GetString("format")
	cols := v.GetStringSlice("cols")
	noHeader := v.GetBool("no-header")
	noRowNum := v.GetBool("no-rownum")
	sortKeys := v.GetStringSlice("sort")
	expandQueryGroup := v.GetBool("expand-query-group")
	showFullPackagePath := v.GetBool("full-package-path")
	opt := QueryOptionFromViper(v)
	if !mapset.NewSet(sortKeys...).IsSubset(mapset.NewSet("file", "function", "type", "table", "sha1")) {
		return errors.Newf("unknown sort key: %s", mapset.NewSet(sortKeys...).Difference(mapset.NewSet("file", "function", "type", "table", "sha1")).ToSlice())
	}
	if !slices.Contains(sortKeys, "file") {
		sortKeys = append(sortKeys, "file")
	}
	if !mapset.NewSet(cols...).IsSubset(mapset.NewSet(headerColumns...)) {
		return errors.Newf("unknown columns: %s", mapset.NewSet(cols...).Difference(mapset.NewSet(headerColumns...)).ToSlice())
	}
	if !slices.Contains([]string{"table", "md", "csv", "tsv", "simple"}, format) {
		return errors.Newf("unknown format: %s", format)
	}

	queryResults, _, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}
	slices.SortFunc(queryResults, sortQuery(sortKeys))

	printOpt := &PrintQueryOption{Cols: defaultHeaderIndex, NoHeader: noHeader, NoRowNum: noRowNum, ExpandQueryGroup: expandQueryGroup, ShowFullPackagePath: showFullPackagePath}
	if len(cols) > 0 {
		printOpt.Cols = make([]int, 0, len(cols))
		for _, col := range cols {
			printOpt.Cols = append(printOpt.Cols, slices.Index(headerColumns, col))
		}
	}

	var p internalio.TablePrinter
	if format == "table" {
		includeRawQuery := printOpt.Cols != nil && slices.Contains(printOpt.Cols, slices.Index(headerColumns, "raw-query"))
		p = internalio.NewTablePrinter(cmd.OutOrStdout(), internalio.WithColWidth(tablewriter.MAX_ROW_WIDTH*4), internalio.WithAutoWrapText(includeRawQuery))
	} else if format == "md" {
		p = internalio.NewMarkdownPrinter(cmd.OutOrStdout())
	} else if format == "simple" {
		p = internalio.NewSimplePrinter(cmd.OutOrStdout())
	} else if format == "csv" {
		p = internalio.NewCSVPrinter(cmd.OutOrStdout())
	} else if format == "tsv" {
		p = internalio.NewTSVPrinter(cmd.OutOrStdout())
	}

	if !printOpt.NoHeader {
		p.SetHeader(makeHeader(printOpt))
	}
	for i, qr := range queryResults {
		for j, q := range qr.Queries() {
			r := append([]string{""}, row(q, qr.Meta, printOpt)...)
			if len(qr.Queries()) > 1 && expandQueryGroup {
				r[0] = fmt.Sprintf("P%d", j+1)
			} else if len(qr.Queries()) > 1 {
				r[0] = "P"
			}
			if !printOpt.NoRowNum {
				r = append([]string{strconv.Itoa(i + 1)}, r...)
			}
			p.AddRow(r)
			if !expandQueryGroup {
				break // only print the first query in the group
			}
		}
	}
	p.Print()
	return nil
}

func sortQuery(sortKeys []string) func(a, b *analysis.QueryResult) int {
	return func(aa, bb *analysis.QueryResult) int {
		return slices.CompareFunc(sortKeys, sortKeys, func(k, _ string) int {
			if k == "type" {
				return int(aa.Queries()[0].Kind) - int(bb.Queries()[0].Kind)
			} else if k == "table" {
				return strings.Compare(aa.Queries()[0].MainTable, bb.Queries()[0].MainTable)
			} else if k == "sha1" {
				return strings.Compare(aa.Queries()[0].Sha(), bb.Queries()[0].Sha())
			} else if k == "function" {
				return strings.Compare(aa.Meta.Func.Name(), bb.Meta.Func.Name())
			} else if k == "file" {
				return aa.Meta.Compare(bb.Meta)
			}
			return 0
		})
	}
}

type PrintQueryOption struct {
	Cols                []int
	NoHeader            bool
	NoRowNum            bool
	ExpandQueryGroup    bool
	ShowFullPackagePath bool
}

func makeHeader(opt *PrintQueryOption) []string {
	header := make([]string, 0, len(opt.Cols)+2)
	if !opt.NoRowNum {
		header = append(header, "#")
	}
	header = append(header, "*")
	for _, col := range opt.Cols {
		header = append(header, strings.ReplaceAll(headerColumns[col], "-", " "))
	}
	return header
}

func row(q *sql.Query, meta *analysis.Meta, opt *PrintQueryOption) []string {
	fullRow := []string{
		meta.Package.Pkg.Name(),
		abbreviatePackagePath(meta.Package.Pkg.Path(), opt),
		analysisutil.FLC(meta.Position()),
		meta.Func.Name(),
		q.Kind.ColoredString(),
		strings.Join(q.Tables, ", "),
		q.Sha(),
		q.String(),
		q.Raw,
	}
	res := make([]string, 0, len(opt.Cols))
	for _, col := range opt.Cols {
		res = append(res, fullRow[col])
	}
	return res
}

var pathDirRegex = regexp.MustCompile(`([^/]+)/`)

func abbreviatePackagePath(path string, opt *PrintQueryOption) string {
	if opt.ShowFullPackagePath {
		return path
	}
	return pathDirRegex.ReplaceAllStringFunc(path, func(m string) string { return m[:1] + "/" })
}
