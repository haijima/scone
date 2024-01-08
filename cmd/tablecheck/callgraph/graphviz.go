package callgraph

import (
	"fmt"
	"strconv"
	"strings"
)

type GraphvizNode struct {
	Name      string
	Shape     string
	Style     string
	Color     string
	FillColor string
	FontSize  string
}

func (n *GraphvizNode) String() string {
	attributes := make([]string, 0)
	if n.Shape != "" {
		attributes = append(attributes, fmt.Sprintf("shape=%q", n.Shape))
	}
	if n.Style != "" {
		attributes = append(attributes, fmt.Sprintf("style=%q", n.Style))
	}
	if n.Color != "" {
		attributes = append(attributes, fmt.Sprintf("color=%q", n.Color))
	}
	if n.FillColor != "" {
		attributes = append(attributes, fmt.Sprintf("fillcolor=%q", n.FillColor))
	}
	if n.FontSize != "" {
		attributes = append(attributes, fmt.Sprintf("fontsize=%q", n.FontSize))
	}
	if len(attributes) == 0 {
		return fmt.Sprintf("\t\"%s\";", n.Name)
	}
	return fmt.Sprintf("\t\"%s\"[%s];", n.Name, strings.Join(attributes, ", "))
}

type GraphvizEdge struct {
	From     string
	To       string
	Style    string
	Color    string
	PenWidth string
	Weight   int
}

func (e *GraphvizEdge) String() string {
	attributes := make([]string, 0)
	if e.Style != "" {
		attributes = append(attributes, fmt.Sprintf("style=%s", e.Style))
	}
	if e.Color != "" {
		attributes = append(attributes, fmt.Sprintf("color=%s", e.Color))
	}
	if e.PenWidth != "" {
		attributes = append(attributes, fmt.Sprintf("penwidth=%s", e.PenWidth))
	}
	if e.Weight != 0 {
		attributes = append(attributes, fmt.Sprintf("weight=%d", e.Weight))
	}
	if len(attributes) == 0 {
		return fmt.Sprintf("\t\"%s\" -> \"%s\";", e.From, e.To)
	}
	return fmt.Sprintf("\t\"%s\" -> \"%s\"[%s];", e.From, e.To, strings.Join(attributes, ", "))
}

func GraphvizRank(rank string, nodes ...string) string {
	n := make([]string, 0, len(nodes))
	for _, node := range nodes {
		n = append(n, strconv.Quote(node))
	}
	return fmt.Sprintf("\t{rank = %s; %s}", rank, strings.Join(n, "; "))
}
