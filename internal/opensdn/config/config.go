// Package config reads objects from the OpenSDN Config API (port 8082).
//
// Every object is fetched with GET /<type>/<uuid> and comes back wrapped as
// {"<type>": {…}}. Links between objects are refs (this object points to
// another), back_refs (another object points to this one) and children.
// Each link carries the target's uuid and fq_name ("to"), so a ref alone is
// often enough and no extra GET is needed.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"plumb/internal/httpx"
)

type Client struct {
	svc *httpx.Service
}

// New sends token as X-Auth-Token when it is non-empty. Deployments that run
// the Config API with keystone auth need it; lab setups without auth ignore it.
func New(hc *httpx.Client, base, token string) *Client {
	h := http.Header{"Accept": {"application/json"}}
	if token != "" {
		h.Set("X-Auth-Token", token)
	}
	return &Client{svc: &httpx.Service{Client: hc, Base: base, Header: h}}
}

type Ref struct {
	To   []string        `json:"to"`
	UUID string          `json:"uuid"`
	Attr json.RawMessage `json:"attr,omitempty"`
}

func (r Ref) FQName() string { return strings.Join(r.To, ":") }

// ImportExport reads the attr of a routing-instance → route-target ref.
// An empty value means the target is both imported and exported.
func (r Ref) ImportExport() string {
	var a struct {
		ImportExport *string `json:"import_export"`
	}
	_ = json.Unmarshal(r.Attr, &a)
	if a.ImportExport == nil || *a.ImportExport == "" {
		return "import+export"
	}
	return *a.ImportExport
}

type Object struct {
	UUID   string   `json:"uuid"`
	FQName []string `json:"fq_name"`
}

func (o Object) Name() string { return strings.Join(o.FQName, ":") }

// VMI is a virtual-machine-interface. Its UUID equals the Neutron port ID.
type VMI struct {
	Object
	VirtualNetworkRefs  []Ref `json:"virtual_network_refs"`
	RoutingInstanceRefs []Ref `json:"routing_instance_refs"`
	SecurityGroupRefs   []Ref `json:"security_group_refs"`
	InstanceIPBackRefs  []Ref `json:"instance_ip_back_refs"`
	FloatingIPBackRefs  []Ref `json:"floating_ip_back_refs"`
}

type VirtualNetwork struct {
	Object
	NetworkID  int `json:"virtual_network_network_id"`
	Properties struct {
		VxlanID        int    `json:"vxlan_network_identifier"`
		ForwardingMode string `json:"forwarding_mode"`
	} `json:"virtual_network_properties"`
	RouteTargetList struct {
		RouteTarget []string `json:"route_target"`
	} `json:"route_target_list"`
	RoutingInstances  []Ref `json:"routing_instances"`
	NetworkPolicyRefs []Ref `json:"network_policy_refs"`
}

type RoutingInstance struct {
	Object
	RouteTargetRefs []Ref `json:"route_target_refs"`
}

type VirtualMachine struct {
	Object
	VirtualRouterBackRefs []Ref `json:"virtual_router_back_refs"`
}

type VirtualRouter struct {
	Object
	IPAddress string `json:"virtual_router_ip_address"`
}

type InstanceIP struct {
	Object
	Address string `json:"instance_ip_address"`
}

type FloatingIP struct {
	Object
	Address string `json:"floating_ip_address"`
}

type BGPRouter struct {
	Object
	Params struct {
		Address    string `json:"address"`
		RouterType string `json:"router_type"`
	} `json:"bgp_router_parameters"`
}

func get[T any](ctx context.Context, c *Client, typ, id string) (*T, error) {
	var wrap map[string]json.RawMessage
	if err := c.svc.GetJSON(ctx, "/"+typ+"/"+id, nil, &wrap); err != nil {
		return nil, err
	}
	raw, ok := wrap[typ]
	if !ok {
		return nil, fmt.Errorf("%s %s: response has no %q key", typ, id, typ)
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", typ, id, err)
	}
	return &v, nil
}

func (c *Client) VMI(ctx context.Context, id string) (*VMI, error) {
	return get[VMI](ctx, c, "virtual-machine-interface", id)
}

func (c *Client) VirtualNetwork(ctx context.Context, id string) (*VirtualNetwork, error) {
	return get[VirtualNetwork](ctx, c, "virtual-network", id)
}

func (c *Client) RoutingInstance(ctx context.Context, id string) (*RoutingInstance, error) {
	return get[RoutingInstance](ctx, c, "routing-instance", id)
}

func (c *Client) VirtualMachine(ctx context.Context, id string) (*VirtualMachine, error) {
	return get[VirtualMachine](ctx, c, "virtual-machine", id)
}

func (c *Client) VirtualRouter(ctx context.Context, id string) (*VirtualRouter, error) {
	return get[VirtualRouter](ctx, c, "virtual-router", id)
}

func (c *Client) InstanceIP(ctx context.Context, id string) (*InstanceIP, error) {
	return get[InstanceIP](ctx, c, "instance-ip", id)
}

func (c *Client) FloatingIP(ctx context.Context, id string) (*FloatingIP, error) {
	return get[FloatingIP](ctx, c, "floating-ip", id)
}

// ControlNodes lists bgp-routers whose router_type is "control-node". Other
// bgp-routers are external peers such as a gateway router.
func (c *Client) ControlNodes(ctx context.Context) ([]BGPRouter, error) {
	var out struct {
		Routers []struct {
			Router BGPRouter `json:"bgp-router"`
		} `json:"bgp-routers"`
	}
	if err := c.svc.GetJSON(ctx, "/bgp-routers", url.Values{"detail": {"true"}}, &out); err != nil {
		return nil, err
	}
	var nodes []BGPRouter
	for _, r := range out.Routers {
		if r.Router.Params.RouterType == "control-node" {
			nodes = append(nodes, r.Router)
		}
	}
	return nodes, nil
}

// VirtualRouters lists every compute node that OpenSDN knows.
func (c *Client) VirtualRouters(ctx context.Context) ([]VirtualRouter, error) {
	var out struct {
		Routers []struct {
			Router VirtualRouter `json:"virtual-router"`
		} `json:"virtual-routers"`
	}
	if err := c.svc.GetJSON(ctx, "/virtual-routers", url.Values{"detail": {"true"}}, &out); err != nil {
		return nil, err
	}
	var vrs []VirtualRouter
	for _, r := range out.Routers {
		vrs = append(vrs, r.Router)
	}
	return vrs, nil
}
