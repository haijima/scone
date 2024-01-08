package query

import (
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strconv"

	"github.com/haijima/scone/internal/tablecheck"
	"github.com/haijima/scone/internal/tablecheck/query"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/tools/go/analysis/passes/buildssa"
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

	ssa, queryResult, err := tablecheck.Analyze(dir, pattern)
	if err != nil {
		return err
	}

	if format == "table" {
		printTable(cmd.OutOrStdout(), queryResult, ssa)
	} else if format == "md" {
		printMarkdown(cmd.OutOrStdout(), queryResult, ssa)
	} else if format == "simple" {
		printSimple(cmd.OutOrStdout(), queryResult, ssa)
	} else if format == "csv" {
		return printCSV(cmd.OutOrStdout(), queryResult, ssa, false)
	} else if format == "tsv" {
		return printCSV(cmd.OutOrStdout(), queryResult, ssa, true)
	} else {
		return fmt.Errorf("unknown format: %s", format)
	}
	return nil
}

func printTable(w io.Writer, queryResult *query.Result, ssa *buildssa.SSA) {
	table := tablewriter.NewWriter(w)
	table.SetColWidth(tablewriter.MAX_ROW_WIDTH * 3)
	printWithTableWriter(table, queryResult, ssa)
}

func printMarkdown(w io.Writer, queryResult *query.Result, ssa *buildssa.SSA) {
	table := tablewriter.NewWriter(w)
	table.SetAutoWrapText(false)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetCenterSeparator("|")
	printWithTableWriter(table, queryResult, ssa)
}

func printSimple(w io.Writer, queryResult *query.Result, ssa *buildssa.SSA) {
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
	printWithTableWriter(table, queryResult, ssa)
}

func printWithTableWriter(w *tablewriter.Table, queryResult *query.Result, ssa *buildssa.SSA) {
	w.SetHeader([]string{"File", "Function", "Type", "Table", "Query"})
	for _, q := range queryResult.Queries {
		pos := ssa.Pkg.Prog.Fset.Position(q.Pos)
		w.Append([]string{
			fmt.Sprintf("%s:%d:%d", filepath.Base(pos.Filename), pos.Line, pos.Column),
			q.Func.Name(),
			q.Kind.String(),
			q.Tables[0],
			q.Raw,
		})
	}
	w.Render()
}

func printCSV(w io.Writer, queryResult *query.Result, ssa *buildssa.SSA, isTSV bool) error {
	writer := csv.NewWriter(w)
	if isTSV {
		writer.Comma = '\t'
	}
	err := writer.Write([]string{"File", "Function", "Type", "Table", "Query"})
	if err != nil {
		return err
	}
	for _, q := range queryResult.Queries {
		pos := ssa.Pkg.Prog.Fset.Position(q.Pos)
		if err := writer.Write([]string{
			fmt.Sprintf("%s:%d:%d", filepath.Base(pos.Filename), pos.Line, pos.Column),
			q.Func.Name(),
			q.Kind.String(),
			q.Tables[0],
			strconv.Quote(q.Raw),
		}); err != nil {
			return err
		}
	}
	writer.Flush()
	return nil
}
