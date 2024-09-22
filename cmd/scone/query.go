package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/analysisutil/ssautil"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/sql"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewQueryCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "query"
	cmd.Aliases = []string{"queries"}
	cmd.Short = "List SQL queries"
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return runQuery(cmd, v) }

	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|simple}")
	cmd.Flags().StringSlice("sort", []string{"file"}, "The sort `keys` {"+strings.Join(sortableColumns, "|")+"}")
	cmd.Flags().StringSlice("cols", []string{}, "The `columns` to show {"+strings.Join(headerColumns, "|")+"}")
	cmd.Flags().Bool("no-header", false, "Hide header")
	cmd.Flags().Bool("no-rownum", false, "Hide row number")
	cmd.Flags().Bool("full-package-path", false, "Show full package path")
	cmd.Flags().Bool("expand-query-group", false, "Expand query group")

	return cmd
}

var headerColumns = []string{"package", "package-path", "file", "function", "type", "tables", "hash", "query", "raw-query"}
var defaultHeaderIndex = []int{0, 1, 2, 3, 4, 5, 6, 7}
var sortableColumns = []string{"file", "function", "type", "tables", "hash"}

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
	filter := v.GetString("filter")
	additionalFuncs := v.GetStringSlice("analyze-funcs")
	if !mapset.NewSet(sortKeys...).IsSubset(mapset.NewSet(sortableColumns...)) {
		return errors.Newf("unknown sort key: %s", mapset.NewSet(sortKeys...).Difference(mapset.NewSet(sortableColumns...)).ToSlice())
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

	queryResults, _, err := analysis.Analyze(cmd.Context(), dir, pattern, analysis.NewOption(filter, additionalFuncs))
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

	t := table.NewWriter()
	t.SetOutputMirror(cmd.OutOrStdout())

	if !printOpt.NoHeader {
		var header table.Row
		if printOpt.NoRowNum {
			header = table.Row{"*"}
		} else {
			header = table.Row{"#", "*"}
		}
		for _, col := range printOpt.Cols {
			header = append(header, strings.ReplaceAll(headerColumns[col], "-", " "))
		}
		t.AppendHeader(header)
	}
	for i, qr := range queryResults {
		for j, q := range qr.Queries() {
			r := slices.Insert(row(q, qr.Posx, printOpt), 0, "")
			if len(qr.Queries()) > 1 && expandQueryGroup {
				r[0] = fmt.Sprintf("P%d", j+1)
			} else if len(qr.Queries()) > 1 {
				r[0] = "P"
			}
			if qr.FromComment {
				r[0] = fmt.Sprintf("%sC", r[0])
			}
			if !printOpt.NoRowNum {
				r = append(table.Row{i + 1}, r...)
			}
			t.AppendRow(r)
			if !expandQueryGroup {
				break // only print the first query in the group
			}
		}
	}

	switch format {
	case "table":
		t.Render()
	case "md":
		t.RenderMarkdown()
	case "csv":
		t.RenderCSV()
	case "tsv":
		t.RenderTSV()
	case "simple":
		t.Style().Options.DrawBorder = false
		t.Style().Options.SeparateHeader = false
		t.Style().Options.SeparateRows = false
		t.Style().Box.MiddleVertical = " "
		t.Render()
	}
	return nil
}

func sortQuery(sortKeys []string) func(a, b *analysis.QueryResult) int {
	return func(aa, bb *analysis.QueryResult) int {
		return slices.CompareFunc(sortKeys, sortKeys, func(k, _ string) int {
			if k == "type" {
				return int(aa.Queries()[0].Kind) - int(bb.Queries()[0].Kind)
			} else if k == "tables" {
				return strings.Compare(aa.Queries()[0].MainTable, bb.Queries()[0].MainTable)
			} else if k == "hash" {
				return strings.Compare(aa.Queries()[0].Hash(), bb.Queries()[0].Hash())
			} else if k == "function" {
				return strings.Compare(aa.Posx.Func.Name(), bb.Posx.Func.Name())
			} else if k == "file" {
				return aa.Posx.Compare(bb.Posx)
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

func row(q *sql.Query, pos *ssautil.Posx, opt *PrintQueryOption) table.Row {
	fullRow := []string{
		pos.Package().Name(),
		pos.PackagePath(!opt.ShowFullPackagePath),
		pos.PositionString(),
		pos.Func.Name(),
		q.Kind.ColoredString(),
		strings.Join(q.Tables, ", "),
		q.Hash(),
		q.String(),
		q.Raw,
	}
	var res table.Row
	for _, col := range opt.Cols {
		res = append(res, fullRow[col])
	}
	return res
}
