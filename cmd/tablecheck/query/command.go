package query

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"

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
	_ = cmd.MarkFlagDirname("dir")

	return cmd
}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	format := v.GetString("format")

	result, err := tablecheck.Analyze(dir, pattern)
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
	w.SetHeader([]string{"File", "Function", "Type", "Table", "Query"})
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
	err := writer.Write([]string{"File", "Function", "Type", "Table", "Query"})
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

func row(q *query.Query) []string {
	file := fmt.Sprintf("%s:%d:%d", filepath.Base(q.Pos.Filename), q.Pos.Line, q.Pos.Column)
	sqlType := q.Kind.String()
	raw := q.Raw
	return []string{
		file,
		q.Func.Name(),
		sqlType,
		q.Tables[0],
		raw,
	}
}
