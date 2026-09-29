// Package sandesh fetches and parses OpenSDN introspect pages.
//
// Introspect is the HTTP debug interface every OpenSDN process exposes
// (control on 8083, vrouter-agent on 8085). A request named FooReq is
// served at /Snh_FooReq and answers with XML like:
//
//	<ItfResp type="sandesh">
//	  <itf_list type="list">
//	    <list type="struct" size="1">
//	      <ItfSandeshData>
//	        <name type="string">tap9c1e4d2b-7a</name>
//
// Field names change between releases, so this package parses into a
// generic tree and lets callers pick fields by name instead of binding the
// whole schema to Go structs.
package sandesh

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/PunGrumpy/plumb/apps/cli/internal/httpx"
)

// Node is one XML element.
type Node struct {
	Name     string
	Type     string // the type attribute: "string", "i32", "list", "struct", …
	Text     string
	Children []*Node
}

// Get calls /Snh_<req> and parses the response.
func Get(ctx context.Context, svc *httpx.Service, req string, params url.Values) (*Node, error) {
	body, err := svc.Get(ctx, "/Snh_"+req, params)
	if err != nil {
		return nil, err
	}
	doc, err := Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", req, err)
	}
	return doc, nil
}

// Parse reads a whole XML document into a tree rooted at a synthetic node.
func Parse(r io.Reader) (*Node, error) {
	dec := xml.NewDecoder(r)
	root := &Node{Name: "#document"}
	stack := []*Node{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		top := stack[len(stack)-1]
		switch t := tok.(type) {
		case xml.StartElement:
			n := &Node{Name: t.Name.Local}
			for _, a := range t.Attr {
				if a.Name.Local == "type" {
					n.Type = a.Value
				}
			}
			top.Children = append(top.Children, n)
			stack = append(stack, n)
		case xml.CharData:
			top.Text += string(t)
		case xml.EndElement:
			top.Text = strings.TrimSpace(top.Text)
			stack = stack[:len(stack)-1]
		}
	}
	return root, nil
}

// Child returns the first direct child called name.
func (n *Node) Child(name string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// Find returns the first descendant called name, depth first.
func (n *Node) Find(name string) *Node {
	if n == nil {
		return nil
	}
	for _, c := range n.Children {
		if c.Name == name {
			return c
		}
		if f := c.Find(name); f != nil {
			return f
		}
	}
	return nil
}

// FindAll returns every descendant called name. It does not look inside a
// match, so nested structs of the same name are not returned twice.
func (n *Node) FindAll(name string) []*Node {
	if n == nil {
		return nil
	}
	var out []*Node
	for _, c := range n.Children {
		if c.Name == name {
			out = append(out, c)
			continue
		}
		out = append(out, c.FindAll(name)...)
	}
	return out
}

// Str returns the text of the direct child called name.
func (n *Node) Str(name string) string {
	if c := n.Child(name); c != nil {
		return c.Text
	}
	return ""
}

// Int returns the direct child called name as an int, or -1.
func (n *Node) Int(name string) int {
	v, err := strconv.Atoi(n.Str(name))
	if err != nil {
		return -1
	}
	return v
}

// Strings returns the <element> values of a list-of-string child.
func (n *Node) Strings(name string) []string {
	var out []string
	for _, e := range n.Child(name).FindAll("element") {
		out = append(out, e.Text)
	}
	return out
}
