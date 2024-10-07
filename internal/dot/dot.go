package dot

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/template"
)

type Graph struct {
	Title    string
	Attrs    Attrs
	Clusters map[string]*Cluster
	Nodes    []*Node
	Edges    []*Edge
	Options  map[string]string
	Ranks    []*Rank
}

type Cluster struct {
	ID       string
	Clusters map[string]*Cluster
	Nodes    []*Node
	Attrs    Attrs
}

func (c *Cluster) String() string {
	return fmt.Sprintf("cluster_%s", c.ID)
}

type Node struct {
	ID    string
	Attrs Attrs
}

func (n *Node) String() string {
	return n.ID
}

type Edge struct {
	From  string
	To    string
	Attrs Attrs
}

func (e *Edge) String() string {
	return fmt.Sprintf("%s -> %s", e.From, e.To)
}

type Attrs map[string]string

func (p Attrs) List() []string {
	l := make([]string, 0, len(p))
	for k, v := range p {
		l = append(l, fmt.Sprintf("%s=%q", k, v))
	}
	return l
}

func (p Attrs) String() string {
	return strings.Join(p.List(), " ")
}

func (p Attrs) Lines() string {
	if len(p) == 0 {
		return ""
	}
	return fmt.Sprintf("%s;", strings.Join(p.List(), ";\n"))
}

type Rank struct {
	Name  string
	Nodes []string
}

func (r *Rank) List() []string {
	l := make([]string, 0, len(r.Nodes))
	for _, v := range r.Nodes {
		l = append(l, fmt.Sprintf("%q", v))
	}
	return l
}

func (r *Rank) String() string {
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

func WriteGraph(w io.Writer, g Graph) error {
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
