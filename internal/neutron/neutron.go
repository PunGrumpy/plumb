// Package neutron reads ports, networks, subnets, security groups and
// floating IPs from the Networking API.
package neutron

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"plumb/internal/httpx"
)

type Client struct {
	svc *httpx.Service
}

// New accepts the catalog URL with or without the /v2.0 suffix.
func New(hc *httpx.Client, endpoint, token string) *Client {
	base := strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(base, "/v2.0") {
		base += "/v2.0"
	}
	return &Client{svc: &httpx.Service{Client: hc, Base: base, Header: http.Header{
		"X-Auth-Token": {token},
		"Accept":       {"application/json"},
	}}}
}

// Port holds the fields plumb reads. binding:vif_type names the backend
// that wired the port: "ovs" for OVS/OVN, "vrouter" for OpenSDN.
type Port struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	NetworkID      string    `json:"network_id"`
	MACAddress     string    `json:"mac_address"`
	Status         string    `json:"status"`
	DeviceID       string    `json:"device_id"`
	DeviceOwner    string    `json:"device_owner"`
	FixedIPs       []FixedIP `json:"fixed_ips"`
	SecurityGroups []string  `json:"security_groups"`
	HostID         string    `json:"binding:host_id"`
	VIFType        string    `json:"binding:vif_type"`
	VNICType       string    `json:"binding:vnic_type"`
	// PortSecurity is nil when the port-security extension is off, which
	// means security groups apply.
	PortSecurity *bool `json:"port_security_enabled"`
}

type Router struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type FixedIP struct {
	SubnetID  string `json:"subnet_id"`
	IPAddress string `json:"ip_address"`
}

type Network struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	MTU            int    `json:"mtu"`
	NetworkType    string `json:"provider:network_type"`
	SegmentationID *int   `json:"provider:segmentation_id"`
}

type Subnet struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CIDR      string `json:"cidr"`
	GatewayIP string `json:"gateway_ip"`
}

type SecurityGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Rules []Rule `json:"security_group_rules"`
}

type Rule struct {
	Direction      string  `json:"direction"`
	EtherType      string  `json:"ethertype"`
	Protocol       *string `json:"protocol"`
	PortRangeMin   *int    `json:"port_range_min"`
	PortRangeMax   *int    `json:"port_range_max"`
	RemoteIPPrefix *string `json:"remote_ip_prefix"`
	RemoteGroupID  *string `json:"remote_group_id"`
}

type FloatingIP struct {
	ID                string `json:"id"`
	FloatingIPAddress string `json:"floating_ip_address"`
	PortID            string `json:"port_id"`
	Status            string `json:"status"`
}

// PortsByFixedIP lists the ports that hold ip as a fixed IP. The same IP
// can exist on several ports when networks in different projects overlap.
func (c *Client) PortsByFixedIP(ctx context.Context, ip string) ([]Port, error) {
	var out struct {
		Ports []Port `json:"ports"`
	}
	err := c.svc.GetJSON(ctx, "/ports", url.Values{"fixed_ips": {"ip_address=" + ip}}, &out)
	return out.Ports, err
}

// FloatingIPsByAddress lists the floating IPs with this address.
func (c *Client) FloatingIPsByAddress(ctx context.Context, ip string) ([]FloatingIP, error) {
	var out struct {
		FloatingIPs []FloatingIP `json:"floatingips"`
	}
	err := c.svc.GetJSON(ctx, "/floatingips", url.Values{"floating_ip_address": {ip}}, &out)
	return out.FloatingIPs, err
}

func (c *Client) Port(ctx context.Context, id string) (*Port, error) {
	var out struct {
		Port Port `json:"port"`
	}
	err := c.svc.GetJSON(ctx, "/ports/"+id, nil, &out)
	return &out.Port, err
}

// routerOwners are the device_owner values of router ports on a tenant
// network: legacy, distributed (DVR) and HA routers.
var routerOwners = []string{"network:router_interface", "network:router_interface_distributed", "network:ha_router_replicated_interface"}

// RouterIDs lists the routers that have an interface on the network.
func (c *Client) RouterIDs(ctx context.Context, networkID string) ([]string, error) {
	var out struct {
		Ports []Port `json:"ports"`
	}
	q := url.Values{"network_id": {networkID}, "device_owner": routerOwners}
	if err := c.svc.GetJSON(ctx, "/ports", q, &out); err != nil {
		return nil, err
	}
	var ids []string
	for _, p := range out.Ports {
		if p.DeviceID != "" && !slices.Contains(ids, p.DeviceID) {
			ids = append(ids, p.DeviceID)
		}
	}
	return ids, nil
}

func (c *Client) Router(ctx context.Context, id string) (*Router, error) {
	var out struct {
		Router Router `json:"router"`
	}
	err := c.svc.GetJSON(ctx, "/routers/"+id, nil, &out)
	return &out.Router, err
}

// PortsByDevice lists the ports whose device_id is the server UUID.
func (c *Client) PortsByDevice(ctx context.Context, deviceID string) ([]Port, error) {
	var out struct {
		Ports []Port `json:"ports"`
	}
	err := c.svc.GetJSON(ctx, "/ports", url.Values{"device_id": {deviceID}}, &out)
	return out.Ports, err
}

func (c *Client) Network(ctx context.Context, id string) (*Network, error) {
	var out struct {
		Network Network `json:"network"`
	}
	err := c.svc.GetJSON(ctx, "/networks/"+id, nil, &out)
	return &out.Network, err
}

func (c *Client) Subnet(ctx context.Context, id string) (*Subnet, error) {
	var out struct {
		Subnet Subnet `json:"subnet"`
	}
	err := c.svc.GetJSON(ctx, "/subnets/"+id, nil, &out)
	return &out.Subnet, err
}

func (c *Client) SecurityGroup(ctx context.Context, id string) (*SecurityGroup, error) {
	var out struct {
		SecurityGroup SecurityGroup `json:"security_group"`
	}
	err := c.svc.GetJSON(ctx, "/security-groups/"+id, nil, &out)
	return &out.SecurityGroup, err
}

func (c *Client) FloatingIPsByPort(ctx context.Context, portID string) ([]FloatingIP, error) {
	var out struct {
		FloatingIPs []FloatingIP `json:"floatingips"`
	}
	err := c.svc.GetJSON(ctx, "/floatingips", url.Values{"port_id": {portID}}, &out)
	return out.FloatingIPs, err
}
