// Package control reads routes and peers from a control node's introspect
// (port 8083). A control node learns a VM's route from the vRouter agent
// over XMPP and reflects it to other agents and BGP peers.
package control

import (
	"context"
	"net/url"

	"plumb/internal/httpx"
	"plumb/internal/opensdn/sandesh"
)

type Client struct {
	svc *httpx.Service
}

func New(hc *httpx.Client, base string) *Client {
	return &Client{svc: &httpx.Service{Client: hc, Base: base}}
}

type Route struct {
	Table  string
	Prefix string
	Paths  []Path
}

// Path is one way to reach a prefix. For a VM route learned from an agent,
// Protocol is XMPP, NextHop is the compute node IP and Label is the MPLS
// label (or VNI) that the compute node assigned to the VM interface.
type Path struct {
	Protocol string
	Source   string
	NextHop  string
	Label    string
	Encap    []string
	OriginVN string
}

// Routes returns the routes for an exact prefix in table, for example
// "default-domain:admin:vn1:vn1.inet.0".
func (c *Client) Routes(ctx context.Context, table, prefix string) ([]Route, error) {
	doc, err := sandesh.Get(ctx, c.svc, "ShowRouteReq", url.Values{
		"routing_table": {table},
		"prefix":        {prefix},
	})
	if err != nil {
		return nil, err
	}
	var out []Route
	for _, t := range doc.FindAll("ShowRouteTable") {
		name := t.Str("routing_table_name")
		for _, r := range t.FindAll("ShowRoute") {
			if r.Str("prefix") != prefix {
				continue
			}
			rt := Route{Table: name, Prefix: prefix}
			for _, p := range r.FindAll("ShowRoutePath") {
				rt.Paths = append(rt.Paths, Path{
					Protocol: p.Str("protocol"),
					Source:   p.Str("source"),
					NextHop:  p.Str("next_hop"),
					Label:    p.Str("label"),
					Encap:    p.Strings("tunnel_encap"),
					OriginVN: p.Str("origin_vn"),
				})
			}
			out = append(out, rt)
		}
	}
	return out, nil
}

// Neighbor is a BGP or XMPP peer. Encoding tells which one.
type Neighbor struct {
	Peer     string
	Address  string
	Encoding string
	State    string
}

func (c *Client) Neighbors(ctx context.Context) ([]Neighbor, error) {
	doc, err := sandesh.Get(ctx, c.svc, "ShowBgpNeighborSummaryReq", nil)
	if err != nil {
		return nil, err
	}
	var out []Neighbor
	for _, n := range doc.FindAll("BgpNeighborResp") {
		out = append(out, Neighbor{
			Peer:     n.Str("peer"),
			Address:  n.Str("peer_address"),
			Encoding: n.Str("encoding"),
			State:    n.Str("state"),
		})
	}
	return out, nil
}
