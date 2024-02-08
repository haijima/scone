package io

import (
	"encoding/csv"
	"io"

	"github.com/olekukonko/tablewriter"
)

type TablePrinter interface {
	SetHeader(header []string)
	AddRow(row []string)
	Print()
}

type tablePrinter struct {
	writer *tablewriter.Table
}

func (t *tablePrinter) SetHeader(header []string) {
	t.writer.SetHeader(header)
}

func (t *tablePrinter) AddRow(row []string) {
	t.writer.Append(row)
}

func (t *tablePrinter) Print() {
	t.writer.Render()
}

type csvPrinter struct {
	writer *csv.Writer
}

func (c *csvPrinter) SetHeader(header []string) {
	_ = c.writer.Write(header)
}

func (c *csvPrinter) AddRow(row []string) {
	_ = c.writer.Write(row)
}

func (c *csvPrinter) Print() {
	c.writer.Flush()
}

type PrinterOpt interface {
	apply(*tablewriter.Table)
}

type printerOptFunc func(*tablewriter.Table)

func (f printerOptFunc) apply(t *tablewriter.Table) { f(t) }

func WithAutoWrapText(b bool) PrinterOpt {
	return printerOptFunc(func(t *tablewriter.Table) { t.SetAutoWrapText(b) })
}

func WithColWidth(w int) PrinterOpt {
	return printerOptFunc(func(t *tablewriter.Table) { t.SetColWidth(w) })
}

func NewTablePrinter(w io.Writer, opts ...PrinterOpt) TablePrinter {
	tw := tablewriter.NewWriter(w)
	tw.SetAutoWrapText(false)
	for _, opt := range opts {
		opt.apply(tw)
	}
	return &tablePrinter{writer: tw}
}

func NewMarkdownPrinter(w io.Writer, opts ...PrinterOpt) TablePrinter {
	tw := tablewriter.NewWriter(w)
	tw.SetAutoWrapText(false)
	tw.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	tw.SetCenterSeparator("|")
	for _, opt := range opts {
		opt.apply(tw)
	}
	return &tablePrinter{writer: tw}
}

func NewSimplePrinter(w io.Writer, opts ...PrinterOpt) TablePrinter {
	tw := tablewriter.NewWriter(w)
	tw.SetAutoWrapText(false)
	tw.SetAutoFormatHeaders(true)
	tw.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	tw.SetAlignment(tablewriter.ALIGN_LEFT)
	tw.SetCenterSeparator("")
	tw.SetColumnSeparator("")
	tw.SetRowSeparator("")
	tw.SetHeaderLine(false)
	tw.SetBorder(false)
	tw.SetTablePadding(" ")
	tw.SetNoWhiteSpace(true)
	for _, opt := range opts {
		opt.apply(tw)
	}
	return &tablePrinter{writer: tw}
}

func NewCSVPrinter(w io.Writer) TablePrinter {
	writer := csv.NewWriter(w)
	return &csvPrinter{writer: writer}
}

func NewTSVPrinter(w io.Writer) TablePrinter {
	writer := csv.NewWriter(w)
	writer.Comma = '\t'
	return &csvPrinter{writer: writer}
}
