package main

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strconv"
	"text/template"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/fatih/color"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/analysisutil"
	internalio "github.com/haijima/scone/internal/io"
	"github.com/haijima/scone/internal/sql"
	"github.com/haijima/scone/internal/util"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewTableCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "table"
	cmd.Short = "List tables information from queries"
	cmd.RunE = func(cmd *cobra.Command, args []string) error { return runTable(cmd, v) }

	cmd.Flags().Bool("summary", false, "Print summary only")
	SetQueryOptionFlags(cmd)

	return cmd
}

func runTable(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	summarizeOnly := v.GetBool("summary")
	opt, err := QueryOptionFromViper(v)
	if err != nil {
		return err
	}

	queryResults, cgs, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}
	return printResult(cmd.OutOrStdout(), queryResults, cgs, PrintTableOption{SummarizeOnly: summarizeOnly})
}

type PrintTableOption struct{ SummarizeOnly bool }

func printResult(w io.Writer, queryResults analysis.QueryResults, cgs map[string]*analysis.CallGraph, opt PrintTableOption) error {
	tableConn := clusterize(queryResults, cgs)
	if err := printSummary(w, queryResults, tableConn); err != nil {
		return err
	}
	if !opt.SummarizeOnly {
		for _, t := range queryResults.AllTables() {
			if err := printTableResult(w, t, queryResults, tableConn); err != nil {
				return err
			}
		}
	}
	fmt.Fprintln(w)
	return nil
}

func clusterize(queryResults analysis.QueryResults, cgs map[string]*analysis.CallGraph) util.Connection {
	c := util.NewConnection(queryResults.AllTableNames()...) // Create a graph with tables as nodes

	// Extract tables updated in the same transaction
	for _, cg := range cgs {
		for _, r := range cg.Nodes {
			if r.IsNotRoot() {
				continue
			}
			// walk from the root nodes
			tablesInTx := mapset.NewSet[string]()
			analysis.Walk(cg, r, func(n *analysis.Node) bool {
				for _, edge := range n.Out {
					if edge.IsQuery() && edge.SqlValue.Kind != sql.Select {
						tablesInTx.Add(edge.Callee)
					}
				}
				return false
			})
			// connect updated tables under the same root node
			util.PairCombinateFunc(tablesInTx.ToSlice(), c.Connect)
		}
	}

	// extract tables used in the same query
	for _, q := range queryResults.AllQueries() {
		util.PairCombinateFunc(q.Tables, c.Connect)
	}
	return c
}

const tmplSummary = `{{title "Summary"}}
  {{key "queries"}}         : {{.queries}}
  {{key "tables"}}          : {{.tables}}
  {{key "cacheability"}}
  {{- range $k, $v := .cacheability}}
	{{colored $k}}	: {{len $v}}	{{printf "%q" $v}}
  {{- end}}
  {{key "table clusters"}}  : {{len .clusters}}
  {{- range .clusters}}
	{{printf "%q" .}}
  {{- end}}
  {{key "partition keys"}}  : found in {{len .partitionKeys}} table(s)
  {{- range $k, $v := .partitionKeys}}
	{{printf "%q for %q" $v $k}}
  {{- end}}
`

func printSummary(w io.Writer, queryResults analysis.QueryResults, tableConn util.Connection) error {
	data := make(map[string]any)
	tables := queryResults.AllTables()

	data["queries"] = len(queryResults)
	data["tables"] = len(tables)
	cacheability := make(map[sql.Cacheability][]string)
	for _, t := range tables {
		cacheability[t.Cacheability()] = append(cacheability[t.Cacheability()], t.Name)
	}
	data["cacheability"] = cacheability
	clusters := make([][]string, 0, len(tableConn.GetClusters()))
	for _, ts := range tableConn.GetClusters() {
		clusters = append(clusters, mapset.Sorted(ts))
	}
	data["clusters"] = clusters
	partitionKeys := make(map[string][]string)
	for _, t := range tables {
		if len(t.PartitionKeys()) > 0 {
			partitionKeys[t.Name] = t.PartitionKeys()
		}
	}
	data["partitionKeys"] = partitionKeys

	return templateRender(w, "summary", tmplSummary, data)
}

const tmplTableResult = `
{{labeled .table .maxKind}}
  {{key "query types"}}	:{{range .kinds}} {{colored .}}{{end}}
  {{key "cacheability"}}	: {{colored .cacheability}}
  {{key "collocation"}}	: {{printf "%q" .collocation}}
  {{key "cluster"}}	: {{printf "%q" .cluster}}
  {{- if .partitionKey}}
  {{key "partition key"}}	: {{printf "%q" .partitionKey}}
  {{- end}}
  {{key "queries"}}	: {{len .queries}}
{{/* Show queries by TablePrinter */}}
`

func printTableResult(w io.Writer, table *sql.Table, queryResults analysis.QueryResults, tableConn util.Connection) error {
	data := make(map[string]any)
	data["table"] = table.Name
	data["maxKind"] = table.MaxKind()
	data["kinds"] = table.Kinds()
	data["cacheability"] = table.Cacheability()
	data["collocation"] = mapset.Sorted(tableConn.GetConnection(table.Name, 1).Difference(mapset.NewSet(table.Name)))
	data["cluster"] = mapset.Sorted(tableConn.GetConnection(table.Name, -1))
	data["partitionKey"] = table.PartitionKeys()
	qrs := make([]*analysis.QueryResult, 0)
	for _, qr := range queryResults {
		for _, q := range qr.Queries() {
			if slices.Contains(q.Tables, table.Name) {
				qrs = append(qrs, qr)
				break
			}
		}
	}
	data["queries"] = qrs

	if err := templateRender(w, "tableResult", tmplTableResult, data); err != nil {
		return err
	}

	// Print queries by TablePrinter
	p := internalio.NewSimplePrinter(w, tablewriter.MAX_ROW_WIDTH*5, true)
	p.SetHeader([]string{"", "#", "file", "function", "t", "query"})
	for i, qr := range qrs {
		for _, q := range qr.Queries() {
			if !slices.Contains(q.Tables, table.Name) {
				continue
			}
			k := "?"
			if q.Kind > sql.Unknown {
				k = q.Kind.Color(q.Kind.String()[:1])
			}
			p.AddRow([]string{"   ", strconv.Itoa(i + 1), analysisutil.FLC(qr.Meta.Position()), qr.Meta.Func.Name(), k, q.Raw})
		}
	}
	return p.Print()
}

var tmplFuncs = map[string]any{
	"labeled": func(table string, kind sql.QueryKind) string {
		c := color.New(color.FgBlack, color.BgWhite)
		switch kind {
		case sql.Select:
			c = color.New(color.FgBlack, color.BgBlue)
		case sql.Insert:
			c = color.New(color.FgBlack, color.BgGreen)
		case sql.Delete:
			c = color.New(color.FgBlack, color.BgRed)
		case sql.Replace, sql.Update:
			c = color.New(color.FgBlack, color.BgYellow)
		}
		return c.Sprintf(" %s ", table)
	},
	"title":   color.CyanString,
	"key":     color.MagentaString,
	"colored": func(s interface{ ColoredString() string }) string { return s.ColoredString() },
}

func templateRender(w io.Writer, name string, tmpl string, data map[string]any) error {
	t, err := template.New(name).Funcs(tmplFuncs).Parse(tmpl)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return err
	}
	_, err = buf.WriteTo(w)
	return err
}
