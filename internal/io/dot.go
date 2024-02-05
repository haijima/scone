package io

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/template"
)

type DotGraph struct {
	Title    string
	Attrs    DotAttrs
	Clusters map[string]*DotCluster
	Nodes    []*DotNode
	Edges    []*DotEdge
	Options  map[string]string
	Ranks    []*DotRank
}

type DotCluster struct {
	ID       string
	Clusters map[string]*DotCluster
	Nodes    []*DotNode
	Attrs    DotAttrs
}

func (c *DotCluster) String() string {
	return fmt.Sprintf("cluster_%s", c.ID)
}

type DotNode struct {
	ID    string
	Attrs DotAttrs
}

func (n *DotNode) String() string {
	return n.ID
}

type DotEdge struct {
	From  string
	To    string
	Attrs DotAttrs
}

func (e *DotEdge) String() string {
	return fmt.Sprintf("%s -> %s", e.From, e.To)
}

type DotAttrs map[string]string

func (p DotAttrs) List() []string {
	l := make([]string, 0, len(p))
	for k, v := range p {
		l = append(l, fmt.Sprintf("%s=%q", k, v))
	}
	return l
}

func (p DotAttrs) String() string {
	return strings.Join(p.List(), " ")
}

func (p DotAttrs) Lines() string {
	if len(p) == 0 {
		return ""
	}
	return fmt.Sprintf("%s;", strings.Join(p.List(), ";\n"))
}

type DotRank struct {
	Name  string
	Nodes []string
}

func (r *DotRank) List() []string {
	l := make([]string, 0, len(r.Nodes))
	for _, v := range r.Nodes {
		l = append(l, fmt.Sprintf("%q", v))
	}
	return l
}

func (r *DotRank) String() string {
	return fmt.Sprintf("{rank = %s; %s}", r.Name, strings.Join(r.List(), "; "))
}

const tmplCluster = `{{define "cluster" -}}
    {{printf "subgraph %q {" .}}
        {{printf "%s" .Attrs.Lines}}
        {{range .Nodes}}
	    {{printf "%q [ %s ]" .ID .Attrs}}
        {{- end}}
        {{range .Clusters}}
        {{template "cluster" .}}
        {{- end}}
    {{println "}" }}
{{- end}}`

const tmplGraph = `digraph scone {
    label="{{.Title}}";
    labeljust="l";
    fontname="Verdana";
    fontsize="14";
    rankdir="LR";
    # bgcolor="lightgray";
    style="solid";
    penwidth="1.0";
    pad="0.0";

    node [fontname="Verdana"];

	{{range .Clusters}}
	{{template "cluster" .}}
	{{- end}}

	{{range .Nodes}}
	{{printf "%q [ %s ]" .ID .Attrs}}
	{{- end}}
    {{- range .Edges}}
	{{printf "%q -> %q [ %s ]" .From .To .Attrs}}
	{{- end}}

	{{range .Ranks}}
	{{.}}	
	{{- end}}
}
`

func WriteDotGraph(w io.Writer, g DotGraph) error {
	t := template.New("dot")
	for _, s := range []string{tmplCluster, tmplGraph} {
		if _, err := t.Parse(s); err != nil {
			return err
		}
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, g); err != nil {
		return err
	}
	_, err := buf.WriteTo(w)
	return err
}
