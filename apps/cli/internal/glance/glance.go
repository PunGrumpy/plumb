// Package glance resolves an image ID to its name.
package glance

import (
	"context"
	"net/http"
	"strings"

	"github.com/PunGrumpy/plumb/apps/cli/internal/httpx"
)

type Client struct {
	svc *httpx.Service
}

func New(hc *httpx.Client, endpoint, token string) *Client {
	base := strings.TrimRight(endpoint, "/")
	if !strings.HasSuffix(base, "/v2") {
		base += "/v2"
	}
	return &Client{svc: &httpx.Service{Client: hc, Base: base, Header: http.Header{
		"X-Auth-Token": {token},
		"Accept":       {"application/json"},
	}}}
}

func (c *Client) ImageName(ctx context.Context, id string) (string, error) {
	var out struct {
		Name string `json:"name"`
	}
	if err := c.svc.GetJSON(ctx, "/images/"+id, nil, &out); err != nil {
		return "", err
	}
	return out.Name, nil
}
