package main

import (
	"bytes"
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

	queryResults, tables, cgs, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}
	return printResult(cmd.OutOrStdout(), queryResults, tables, cgs, PrintTableOption{SummarizeOnly: summarizeOnly})
}

type PrintTableOption struct{ SummarizeOnly bool }

func printResult(w io.Writer, queryResults []*analysis.QueryResult, tables mapset.Set[string], cgs map[string]*analysis.CallGraph, opt PrintTableOption) error {
	filterColumns := util.NewSetMap[string, string]()
	kindsMap := util.NewSetMap[string, sql.QueryKind]()
	for _, qr := range queryResults {
		for _, q := range qr.Queries() {
			for t, cols := range q.FilterColumnMap {
				filterColumns.Intersect(t, cols)
			}
			for _, t := range q.Tables {
				kindsMap.Add(t, q.Kind)
			}
		}
	}
	tableConn := clusterize(tables, queryResults, cgs)

	if err := printSummary(w, tables, queryResults, tableConn, filterColumns, kindsMap); err != nil {
		return err
	}
	if !opt.SummarizeOnly {
		for _, t := range mapset.Sorted(tables) {
			if err := printTableResult(w, t, queryResults, tableConn, filterColumns[t], kindsMap[t]); err != nil {
				return err
			}
		}
	}
	return nil
}

func clusterize(tables mapset.Set[string], queryResults []*analysis.QueryResult, cgs map[string]*analysis.CallGraph) util.Connection {
	c := util.NewConnection(tables.ToSlice()...) // Create a graph with tables as nodes

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
	hard coded    : {{len .hardcoded}}	{{printf "%q" .hardcoded}}
	read-through  : {{len .readThrough}}	{{printf "%q" .readThrough}}
	write-through : {{len .writeThrough}}	{{printf "%q" .writeThrough}}
  {{key "table clusters"}}  : {{len .clusters}}
  {{- range .clusters}}
	{{printf "%q" .}}
  {{- end}}
  {{key "partition keys"}}  : found in {{len .partitionKeys}} table(s)
  {{- range $key, $value := .partitionKeys}}
	{{printf "%q for %q" $value $key}}
  {{- end}}
`

func printSummary(w io.Writer, tables mapset.Set[string], queryGroups []*analysis.QueryResult, tableConn util.Connection, filterColumns map[string]mapset.Set[string], kindsMap map[string]mapset.Set[sql.QueryKind]) error {
	data := make(map[string]any)

	data["queries"] = len(queryGroups)
	data["tables"] = tables.Cardinality()
	var hardCoded, readThrough, writeThrough []string
	for _, t := range mapset.Sorted(tables) {
		switch slices.Max(kindsMap[t].ToSlice()) {
		case sql.Select:
			hardCoded = append(hardCoded, t)
		case sql.Insert:
			readThrough = append(readThrough, t)
		case sql.Delete, sql.Replace, sql.Update:
			writeThrough = append(writeThrough, t)
		}
	}
	data["hardcoded"] = hardCoded
	data["readThrough"] = readThrough
	data["writeThrough"] = writeThrough
	clusters := make([][]string, 0, len(tableConn.GetClusters()))
	for _, ts := range tableConn.GetClusters() {
		clusters = append(clusters, mapset.Sorted(ts))
	}
	data["clusters"] = clusters
	partitionKeys := make(map[string][]string)
	for t, cols := range filterColumns {
		if cols.Cardinality() > 0 {
			partitionKeys[t] = mapset.Sorted(cols)
		}
	}
	data["partitionKeys"] = partitionKeys

	funcs := make(map[string]interface{})
	funcs["title"] = color.CyanString
	funcs["key"] = color.MagentaString
	t, err := template.New("summary").Funcs(funcs).Parse(tmplSummary)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	_, err = buf.WriteTo(w)
	return err
}

var tmplTableResult = `
{{title .table .maxKind}}
  {{key "query types"}}	:{{range .kinds}} {{.}}{{end}}
  {{key "cacheability"}}	: {{.cacheability}}
  {{key "collocation"}}	: {{printf "%q" .collocation}}
  {{key "cluster"}}	: {{printf "%q" .cluster}}
  {{- if .partitionKey}}
  {{key "partition key"}}	: {{printf "%q" .partitionKey}}
  {{- end}}
  {{key "queries"}}	: {{len .queries}}
{{/* Show queries by TablePrinter */}}
`

func printTableResult(w io.Writer, table string, queryResults []*analysis.QueryResult, tableConn util.Connection, filterColumns mapset.Set[string], kinds mapset.Set[sql.QueryKind]) error {
	data := make(map[string]any)
	data["table"] = table
	data["maxKind"] = slices.Max(kinds.ToSlice())
	data["kinds"] = make([]string, 0, kinds.Cardinality())
	for _, k := range mapset.Sorted(kinds) {
		data["kinds"] = append(data["kinds"].([]string), k.Color(k.String()))
	}
	data["cacheability"] = slices.Max(kinds.ToSlice()).Color(slices.Max(kinds.ToSlice()).String())
	switch maxKind := slices.Max(kinds.ToSlice()); maxKind {
	case sql.Select:
		data["cacheability"] = maxKind.Color("Hard coded")
	case sql.Insert:
		data["cacheability"] = maxKind.Color("Read-through")
	case sql.Delete, sql.Replace, sql.Update:
		data["cacheability"] = "Write-through"
	case sql.Unknown:
		data["cacheability"] = maxKind.Color("Unknown")
	}
	data["collocation"] = mapset.Sorted(tableConn.GetConnection(table, 1).Difference(mapset.NewSet(table)))
	data["cluster"] = mapset.Sorted(tableConn.GetConnection(table, -1))
	data["partitionKey"] = mapset.Sorted(filterColumns)
	qrs := make([]*analysis.QueryResult, 0)
	for _, qr := range queryResults {
		for _, q := range qr.Queries() {
			if slices.Contains(q.Tables, table) {
				qrs = append(qrs, qr)
				break
			}
		}
	}
	data["queries"] = qrs

	funcs := make(map[string]interface{})
	funcs["key"] = color.MagentaString
	funcs["title"] = func(table string, kind sql.QueryKind) string {
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
	}
	t, err := template.New("tableResult").Funcs(funcs).Parse(tmplTableResult)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	_, err = buf.WriteTo(w)
	if err != nil {
		return err
	}

	// Print queries by TablePrinter
	p := internalio.NewSimplePrinter(w, tablewriter.MAX_ROW_WIDTH*5, true)
	p.SetHeader([]string{"", "#", "file", "function", "t", "query"})
	for i, qr := range qrs {
		for _, q := range qr.Queries() {
			k := "?"
			if q.Kind > sql.Unknown {
				k = q.Kind.Color(q.Kind.String()[:1])
			}
			p.AddRow([]string{"   ", strconv.Itoa(i + 1), analysisutil.FLC(qr.Meta.Position()), qr.Meta.Func.Name(), k, q.Raw})
		}
	}
	return p.Print()
}
