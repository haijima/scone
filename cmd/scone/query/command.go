package query

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/haijima/scone/cmd/scone/option"
	"github.com/haijima/scone/internal/analysis"
	"github.com/haijima/scone/internal/analysis/query"
	internalio "github.com/haijima/scone/internal/io"
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

	cmd.Flags().String("format", "table", "The output format {table|md|csv|tsv|simple}")
	cmd.Flags().StringSlice("sort", []string{"file"}, "The sort `keys` {file|function|type|table|sha1}")
	cmd.Flags().StringSlice("cols", []string{}, "The `columns` to show {"+strings.Join(headerColumns, "|")+"}")
	cmd.Flags().Bool("no-header", false, "Hide header")
	cmd.Flags().Bool("no-rownum", false, "Hide row number")
	cmd.Flags().Bool("full-package-path", false, "Show full package path")
	option.SetQueryOptionFlags(cmd)

	return cmd
}

var headerColumns = []string{"package", "package-path", "file", "function", "type", "table", "related-tables", "sha1", "query", "raw-query"}
var defaultHeaderIndex = []int{0, 1, 2, 3, 4, 5, 6, 7, 8}

func run(cmd *cobra.Command, v *viper.Viper) error {
	dir := v.GetString("dir")
	pattern := v.GetString("pattern")
	format := v.GetString("format")
	cols := v.GetStringSlice("cols")
	noHeader := v.GetBool("no-header")
	noRowNum := v.GetBool("no-rownum")
	sortKeys := v.GetStringSlice("sort")
	showFullPackagePath := v.GetBool("full-package-path")
	opt, err := option.QueryOptionFromViper(v)
	if err != nil {
		return err
	}

	queries, _, _, err := analysis.Analyze(dir, pattern, opt)
	if err != nil {
		return err
	}

	if !mapset.NewSet(sortKeys...).IsSubset(mapset.NewSet("file", "function", "type", "table", "sha1")) {
		return fmt.Errorf("unknown sort key: %s", mapset.NewSet(sortKeys...).Difference(mapset.NewSet("file", "function", "type", "table", "sha1")).ToSlice())
	}
	if !slices.Contains(sortKeys, "file") {
		sortKeys = append(sortKeys, "file")
	}
	slices.SortFunc(queries, sortQuery(sortKeys))

	printOpt := &PrintOption{Cols: defaultHeaderIndex, NoHeader: noHeader, NoRowNum: noRowNum, ShowFullPackagePath: showFullPackagePath}
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
	pkgs := mapset.NewSet[string]()
	for _, q := range queries {
		pkgs.Add(q.Package.Pkg.Path())
	}
	printOpt.pkgBasePath = findCommonPrefix(pkgs.ToSlice())

	var p internalio.TablePrinter
	if format == "table" {
		maxWidth := tablewriter.MAX_ROW_WIDTH * 4
		includeRawQuery := printOpt.Cols != nil && slices.Contains(printOpt.Cols, 9)
		p = internalio.NewTablePrinter(cmd.OutOrStdout(), maxWidth, includeRawQuery)
	} else if format == "md" {
		p = internalio.NewMarkdownPrinter(cmd.OutOrStdout())
	} else if format == "simple" {
		p = internalio.NewSimplePrinter(cmd.OutOrStdout(), tablewriter.MAX_ROW_WIDTH, false)
	} else if format == "csv" {
		p = internalio.NewCSVPrinter(cmd.OutOrStdout())
	} else if format == "tsv" {
		p = internalio.NewTSVPrinter(cmd.OutOrStdout())
	} else {
		return fmt.Errorf("unknown format: %s", format)
	}

	if !printOpt.NoHeader {
		p.SetHeader(makeHeader(printOpt))
	}
	for i, q := range queries {
		r := row(q, printOpt)
		if !printOpt.NoRowNum {
			r = append([]string{strconv.Itoa(i + 1)}, r...)
		}
		p.AddRow(r)
	}
	return p.Print()
}

func sortQuery(sortKeys []string) func(a *query.Query, b *query.Query) int {
	return func(a, b *query.Query) int {
		for _, sortKey := range sortKeys {
			if sortKey == "function" && a.Func.Name() != b.Func.Name() {
				return strings.Compare(a.Func.Name(), b.Func.Name())
			} else if sortKey == "type" && a.Kind != b.Kind {
				return int(a.Kind) - int(b.Kind)
			} else if sortKey == "table" && a.MainTable != b.MainTable {
				return strings.Compare(a.MainTable, b.MainTable)
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
	}
}

type PrintOption struct {
	Cols                []int
	NoHeader            bool
	NoRowNum            bool
	ShowFullPackagePath bool
	pkgBasePath         string
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
	pkgPath := q.Package.Pkg.Path()
	if !opt.ShowFullPackagePath && opt.pkgBasePath != "" && strings.HasPrefix(pkgPath, opt.pkgBasePath) {
		pkgPath = strings.TrimPrefix(pkgPath, opt.pkgBasePath)
		pkgPath = shortenPackagePath(opt.pkgBasePath) + pkgPath
	}

	sqlType := q.Kind.ColoredString()

	raw := q.Raw
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

	var tables string
	if len(q.Tables) > 0 {
		tables = strings.Join(q.Tables[1:], ", ")
	}

	fullRow := []string{
		q.Package.Pkg.Name(),
		pkgPath,
		q.FLC(),
		q.Func.Name(),
		sqlType,
		q.MainTable,
		tables,
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

func findCommonPrefix(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	prefix := strs[0]
	for _, str := range strs {
		for len(str) < len(prefix) || str[:len(prefix)] != prefix {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return prefix
}

func shortenPackagePath(path string) string {
	re := regexp.MustCompile(`([^/]+)/`)
	return re.ReplaceAllStringFunc(path, func(m string) string {
		return string(m[0]) + "/"
	})
}
