package io

import (
	"encoding/csv"
	"io"

	"github.com/olekukonko/tablewriter"
)

type TablePrinter interface {
	SetHeader(header []string)
	AddRow(row []string)
	Print() error
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

func (t *tablePrinter) Print() error {
	t.writer.Render()
	return nil
}

type csvPrinter struct {
	writer *csv.Writer
}

func (c *csvPrinter) SetHeader(header []string) {
	c.writer.Write(header)
}

func (c *csvPrinter) AddRow(row []string) {
	c.writer.Write(row)
}

func (c *csvPrinter) Print() error {
	c.writer.Flush()
	return nil
}

func NewTablePrinter(w io.Writer, rowWidth int, autoWrap bool) TablePrinter {
	tw := tablewriter.NewWriter(w)
	tw.SetColWidth(rowWidth)
	tw.SetAutoWrapText(autoWrap)
	return &tablePrinter{writer: tw}
}

func NewMarkdownPrinter(w io.Writer) TablePrinter {
	tw := tablewriter.NewWriter(w)
	tw.SetAutoWrapText(false)
	tw.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	tw.SetCenterSeparator("|")
	return &tablePrinter{writer: tw}
}

func NewSimplePrinter(w io.Writer, rowWidth int, autoWrap bool) TablePrinter {
	tw := tablewriter.NewWriter(w)
	tw.SetColWidth(rowWidth)
	tw.SetAutoWrapText(autoWrap)
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
