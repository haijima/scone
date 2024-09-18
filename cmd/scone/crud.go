package main

import (
	"io"
	"slices"
	"strings"

	"github.com/haijima/epf"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/sql"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewCrudCmd(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "crud"
	cmd.Short = "Show the CRUD operations for each endpoint"
	cmd.Args = cobra.NoArgs
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runCrud(cmd, v)
	}

	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|html|simple}")

	return cmd
}

func runCrud(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	filter := v.GetString("filter")
	format := v.GetString("format")
	additionalFuncs := v.GetStringSlice("analyze-funcs")

	_, cgs, err := analysis.Analyze(cmd.Context(), dir, pattern, analysis.NewOption(filter, additionalFuncs))
	if err != nil {
		return err
	}

	ext, err := epf.AutoExtractor(dir, pattern)
	if err != nil {
		return err
	}
	endpoints, err := epf.FindEndpoints(dir, pattern, ext)
	if err != nil {
		return err
	}

	printCrud(cmd.OutOrStdout(), endpoints, cgs, format)
	return nil
}

func printCrud(w io.Writer, endpoints []*epf.Endpoint, cgs map[string]*analysis.CallGraph, format string) {
	crud := make(map[string]map[string]string)
	for _, ep := range endpoints {
		for _, cg := range cgs {
			for _, node := range cg.Nodes {
				if node.Name == ep.FuncName {
					crud[ep.FuncName] = make(map[string]string)
					for _, c := range search(cg, node) {
						crud[ep.FuncName][c.Table] += c.Kind.CRUD()
					}
				}
			}
		}
	}
	slices.SortFunc(endpoints, func(i, j *epf.Endpoint) int { return strings.Compare(i.Path, j.Path) })

	tables := make([]string, 0)
	for _, cg := range cgs {
		for _, node := range cg.Nodes {
			if node.IsTable() {
				tables = append(tables, node.Name)
			}
		}
	}
	slices.Sort(tables)

	t := table.NewWriter()
	t.SetOutputMirror(w)
	var header table.Row
	header = append(header, "METHOD", "URI", "Function")
	for _, table := range tables {
		header = append(header, table)
	}
	t.AppendHeader(header)
	for _, ep := range endpoints {
		var row table.Row
		row = append(row, ep.Method)
		row = append(row, ep.Path)
		row = append(row, ep.FuncName)
		for _, tbl := range tables {
			if kind, ok := crud[ep.FuncName][tbl]; ok {
				var v string
				for _, k := range []string{"C", "R", "U", "D", "?"} {
					if strings.Contains(kind, k) {
						v += k
					}
				}
				row = append(row, v)
			} else {
				row = append(row, "")
			}
		}
		t.AppendRow(row)
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
	case "html":
		t.RenderHTML()
	case "simple":
		t.Style().Options.DrawBorder = false
		t.Style().Options.SeparateHeader = false
		t.Style().Options.SeparateRows = false
		t.Style().Box.MiddleVertical = " "
		t.Render()
	}
}

type Crud struct {
	Kind  sql.QueryKind
	Table string
}

func search(cg *analysis.CallGraph, node *analysis.Node) []Crud {
	results := make([]Crud, 0)
	for _, edge := range node.Out {
		if edge.IsQuery() {
			results = append(results, Crud{Kind: edge.SqlValue.Kind, Table: edge.Callee})
		} else {
			results = append(results, search(cg, cg.Nodes[edge.Callee])...)
		}
	}
	return results
}
