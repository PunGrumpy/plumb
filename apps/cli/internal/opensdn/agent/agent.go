// Package agent reads interfaces, VRFs, routes and flows from the vRouter
// agent introspect (port 8085) on a compute node.
package agent

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/PunGrumpy/plumb/apps/cli/internal/httpx"
	"github.com/PunGrumpy/plumb/apps/cli/internal/opensdn/sandesh"
)

type Client struct {
	svc *httpx.Service
}

func New(hc *httpx.Client, base string) *Client {
	return &Client{svc: &httpx.Service{Client: hc, Base: base}}
}

// Interface is the agent's view of a VMI: the tap device, the VRF it is
// placed in and the label other computes use to reach it.
type Interface struct {
	Name   string
	VRF    string
	Active string
	Label  string
}

func (c *Client) Interface(ctx context.Context, uuid string) (*Interface, error) {
	doc, err := sandesh.Get(ctx, c.svc, "ItfReq", url.Values{"uuid": {uuid}})
	if err != nil {
		return nil, err
	}
	for _, n := range doc.FindAll("ItfSandeshData") {
		if n.Str("uuid") != uuid {
			continue
		}
		return &Interface{
			Name:   n.Str("name"),
			VRF:    n.Str("vrf_name"),
			Active: n.Str("active"),
			Label:  n.Str("label"),
		}, nil
	}
	return nil, fmt.Errorf("agent has no interface %s", uuid)
}

type VRF struct {
	Name    string
	UCIndex int
}

// VRF looks up the unicast table index that route queries need.
func (c *Client) VRF(ctx context.Context, name string) (*VRF, error) {
	doc, err := sandesh.Get(ctx, c.svc, "VrfListReq", url.Values{"name": {name}})
	if err != nil {
		return nil, err
	}
	for _, n := range doc.FindAll("VrfSandeshData") {
		if n.Str("name") == name {
			return &VRF{Name: name, UCIndex: n.Int("ucindex")}, nil
		}
	}
	return nil, fmt.Errorf("agent has no VRF %s", name)
}

type Route struct {
	Prefix string
	Paths  []Path
}

// Path is one forwarding entry. NHType "interface" means the VM is local;
// "tunnel" means the packet is encapsulated towards TunnelDst.
type Path struct {
	Peer       string
	Label      string
	NHType     string
	Interface  string
	TunnelDst  string
	TunnelType string
}

func (c *Client) Routes(ctx context.Context, vrfIndex int, ip string) ([]Route, error) {
	doc, err := sandesh.Get(ctx, c.svc, "Inet4UcRouteReq", url.Values{
		"vrf_index":  {strconv.Itoa(vrfIndex)},
		"src_ip":     {ip},
		"prefix_len": {"32"},
	})
	if err != nil {
		return nil, err
	}
	var out []Route
	for _, r := range doc.FindAll("RouteUcSandeshData") {
		if r.Str("src_ip") != ip {
			continue
		}
		rt := Route{Prefix: ip + "/" + r.Str("src_plen")}
		for _, p := range r.FindAll("PathSandeshData") {
			nh := p.Find("NhSandeshData")
			rt.Paths = append(rt.Paths, Path{
				Peer:       p.Str("peer"),
				Label:      p.Str("label"),
				NHType:     nh.Str("type"),
				Interface:  nh.Str("itf"),
				TunnelDst:  nh.Str("dip"),
				TunnelType: nh.Str("tunnel_type"),
			})
		}
		out = append(out, rt)
	}
	return out, nil
}

// XMPPPeer is one control node this agent talks to.
type XMPPPeer struct {
	Controller string
	State      string
	Config     bool
}

func (c *Client) XMPPPeers(ctx context.Context) ([]XMPPPeer, error) {
	doc, err := sandesh.Get(ctx, c.svc, "AgentXmppConnectionStatusReq", nil)
	if err != nil {
		return nil, err
	}
	var out []XMPPPeer
	for _, n := range doc.FindAll("AgentXmppData") {
		out = append(out, XMPPPeer{
			Controller: n.Str("controller_ip"),
			State:      n.Str("state"),
			Config:     n.Str("cfg_controller") == "Yes",
		})
	}
	return out, nil
}

type Flow struct {
	SrcIP      string
	DstIP      string
	SrcPort    string
	DstPort    string
	Protocol   string
	DropReason string
}

// Flows returns the first page of the flow table. The agent pages large
// tables, so the result is a sample, not a full count.
func (c *Client) Flows(ctx context.Context) ([]Flow, error) {
	doc, err := sandesh.Get(ctx, c.svc, "FetchAllFlowRecords", nil)
	if err != nil {
		return nil, err
	}
	var out []Flow
	for _, n := range doc.FindAll("SandeshFlowData") {
		out = append(out, Flow{
			SrcIP:      n.Str("sip"),
			DstIP:      n.Str("dip"),
			SrcPort:    n.Str("src_port"),
			DstPort:    n.Str("dst_port"),
			Protocol:   n.Str("protocol"),
			DropReason: n.Str("drop_reason"),
		})
	}
	return out, nil
}
