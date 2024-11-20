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
	"github.com/haijima/scone/internal/sql"
	"github.com/haijima/scone/internal/util"
	prettyTable "github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func NewTableCommand(v *viper.Viper, _ afero.Fs) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Use = "table"
	cmd.Aliases = []string{"tables"}
	cmd.Short = "List tables information from queries"
	cmd.RunE = func(cmd *cobra.Command, _ []string) error { return runTable(cmd, v) }

	cmd.Flags().Bool("summary", false, "Print summary only")
	cmd.Flags().Bool("collapse-phi", false, "Collapse phi queries")

	return cmd
}

func runTable(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	summaryOnly := v.GetBool("summary")
	collapsePhi := v.GetBool("collapse-phi")
	filter := v.GetString("filter")
	additionalFuncs := v.GetStringSlice("analyze-funcs")

	queryResults, cgs, err := analysis.Analyze(cmd.Context(), dir, pattern, analysis.NewOption(filter, additionalFuncs))
	if err != nil {
		return err
	}
	tableConn := clusterize(queryResults, cgs)

	if err := printSummary(cmd.OutOrStdout(), queryResults, tableConn); err != nil {
		return err
	}
	if !summaryOnly {
		for _, t := range queryResults.AllTables() {
			if err := printTableResult(cmd.OutOrStdout(), t, queryResults, tableConn, collapsePhi); err != nil {
				return err
			}
		}
	}
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

func clusterize(queryResults analysis.QueryResults, cgs map[string]*analysis.CallGraph) util.Connection {
	c := util.NewConnection(queryResults.AllTableNames()...) // Create a graph with tables as nodes

	// Extract tables updated in the same transaction
	for _, cg := range cgs {
		for _, r := range cg.Nodes {
			if r.IsRoot() {
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
	}

	// extract tables used in the same query
	for _, qr := range queryResults {
		for _, q := range qr.Queries() {
			util.PairCombinateFunc(q.Tables, c.Connect)
		}
	}
	return c
}

const tmplSummary = `{{title "Summary"}}
  {{key "queries"}}         : {{.queries}}
  {{key "tables"}}          : {{.tables}}
  {{key "cacheability"}}
  {{- range $k, $v := .cacheability}}
	{{printf "%-19s" (colored $k)}}: {{len $v}}	{{printf "%q" $v}}
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
	tables := queryResults.AllTables()
	cacheability := make(map[sql.Cacheability][]string)
	partitionKeys := make(map[string][]string)
	for _, t := range tables {
		cacheability[t.Cacheability()] = append(cacheability[t.Cacheability()], t.Name)
		if len(t.PartitionKeys()) > 0 {
			partitionKeys[t.Name] = t.PartitionKeys()
		}
	}
	clusters := make([][]string, 0, len(tableConn.GetClusters()))
	for _, ts := range tableConn.GetClusters() {
		clusters = append(clusters, mapset.Sorted(ts))
	}

	data := make(map[string]any)
	data["queries"] = len(queryResults)
	data["tables"] = len(tables)
	data["cacheability"] = cacheability
	data["clusters"] = clusters
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

func printTableResult(w io.Writer, table *sql.Table, queryResults analysis.QueryResults, tableConn util.Connection, collapsePhi bool) error {
	qrs := make([]*analysis.QueryResult, 0)
	for _, qr := range queryResults {
		if slices.ContainsFunc(qr.Queries(), func(q *sql.Query) bool { return slices.Contains(q.Tables, table.Name) }) {
			qrs = append(qrs, qr)
		}
	}

	data := make(map[string]any)
	data["table"] = table.Name
	data["maxKind"] = table.MaxKind()
	data["kinds"] = table.Kinds()
	data["cacheability"] = table.Cacheability()
	data["collocation"] = mapset.Sorted(tableConn.GetConnection(table.Name, 1).Difference(mapset.NewSet(table.Name)))
	data["cluster"] = mapset.Sorted(tableConn.GetConnection(table.Name, -1))
	data["partitionKey"] = table.PartitionKeys()
	data["queries"] = qrs

	if err := templateRender(w, "tableResult", tmplTableResult, data); err != nil {
		return err
	}

	// Print queries
	t := prettyTable.NewWriter()
	t.SetOutputMirror(w)
	t.Style().Options.DrawBorder = false
	t.Style().Options.SeparateHeader = false
	t.Style().Options.SeparateRows = false
	t.Style().Box.MiddleVertical = " "
	t.AppendHeader(prettyTable.Row{"", "#", "file", "function", "t", "query"})
	for i, qr := range qrs {
		for _, q := range qr.Queries() {
			if slices.Contains(q.Tables, table.Name) {
				t.AppendRow(prettyTable.Row{"   ", strconv.Itoa(i + 1), qr.Posx.PositionString(), qr.Posx.Func.Name(), q.Kind.Color(q.Kind.CRUD()), q.Raw}) //nolint:govet
				if collapsePhi {
					break
				}
			}
		}
	}
	t.Render()
	return nil
}

var tmplFuncs = map[string]any{
	"labeled": func(table string, kind sql.QueryKind) string {
		return color.New(color.FgBlack, kind.ColorAttribute()+10).Sprintf(" %s ", table)
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
