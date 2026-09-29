// Package nova reads a server and its port attachments from the Compute API.
package nova

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"

	"plumb/internal/httpx"
)

// Microversion 2.47 embeds the flavor in the server body, which saves a
// call to /flavors and still works after the flavor is deleted.
const microversion = "2.47"

type Client struct {
	svc *httpx.Service
}

func New(hc *httpx.Client, endpoint, token string) *Client {
	return &Client{svc: &httpx.Service{Client: hc, Base: endpoint, Header: http.Header{
		"X-Auth-Token":          {token},
		"Accept":                {"application/json"},
		"OpenStack-API-Version": {"compute " + microversion},
	}}}
}

// Server holds the fields plumb reads. The OS-EXT-SRV-ATTR fields are
// empty unless the token has the admin role.
type Server struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Host         string `json:"OS-EXT-SRV-ATTR:host"`
	InstanceName string `json:"OS-EXT-SRV-ATTR:instance_name"`
	Flavor       Flavor `json:"flavor"`
	Image        Image  `json:"image"`
}

type Flavor struct {
	Name  string `json:"original_name"`
	VCPUs int    `json:"vcpus"`
	RAM   int    `json:"ram"`
	Disk  int    `json:"disk"`
}

// Image is {"id": …} or "" when the server boots from a volume.
type Image struct {
	ID string
}

func (i *Image) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		*i = Image{}
		return nil
	}
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	i.ID = v.ID
	return nil
}

// Interface is one entry of /servers/{id}/os-interface.
type Interface struct {
	PortID   string    `json:"port_id"`
	MACAddr  string    `json:"mac_addr"`
	FixedIPs []FixedIP `json:"fixed_ips"`
}

type FixedIP struct {
	SubnetID  string `json:"subnet_id"`
	IPAddress string `json:"ip_address"`
}

// FindByName lists servers whose name is exactly name. Nova treats the name
// filter as a regular expression, so the name is quoted and anchored.
// allProjects searches every project, which only an admin token may do.
func (c *Client) FindByName(ctx context.Context, name string, allProjects bool) ([]Server, error) {
	var out struct {
		Servers []Server `json:"servers"`
	}
	q := url.Values{"name": {"^" + regexp.QuoteMeta(name) + "$"}}
	if allProjects {
		q.Set("all_tenants", "1")
	}
	if err := c.svc.GetJSON(ctx, "/servers", q, &out); err != nil {
		return nil, err
	}
	return out.Servers, nil
}

func (c *Client) Server(ctx context.Context, id string) (*Server, error) {
	var out struct {
		Server Server `json:"server"`
	}
	if err := c.svc.GetJSON(ctx, "/servers/"+id, nil, &out); err != nil {
		return nil, err
	}
	return &out.Server, nil
}

func (c *Client) Interfaces(ctx context.Context, id string) ([]Interface, error) {
	var out struct {
		Attachments []Interface `json:"interfaceAttachments"`
	}
	if err := c.svc.GetJSON(ctx, "/servers/"+id+"/os-interface", nil, &out); err != nil {
		return nil, err
	}
	return out.Attachments, nil
}
