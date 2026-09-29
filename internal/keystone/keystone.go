// Package keystone gets a scoped token and reads the service catalog, which
// is how every other OpenStack client finds its endpoint.
package keystone

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"plumb/internal/httpx"
)

// Credentials mirror the OS_* variables that openrc exports.
type Credentials struct {
	AuthURL           string
	Username          string
	Password          string
	UserDomainName    string
	UserDomainID      string
	ProjectName       string
	ProjectID         string
	ProjectDomainName string
	ProjectDomainID   string
	AppCredID         string
	AppCredSecret     string
	Region            string
	Interface         string
}

// CredentialsFromEnv reads the OS_* variables through getenv.
func CredentialsFromEnv(getenv func(string) string) Credentials {
	first := func(keys ...string) string {
		for _, k := range keys {
			if v := getenv(k); v != "" {
				return v
			}
		}
		return ""
	}
	return Credentials{
		AuthURL:           getenv("OS_AUTH_URL"),
		Username:          getenv("OS_USERNAME"),
		Password:          getenv("OS_PASSWORD"),
		UserDomainName:    getenv("OS_USER_DOMAIN_NAME"),
		UserDomainID:      getenv("OS_USER_DOMAIN_ID"),
		ProjectName:       first("OS_PROJECT_NAME", "OS_TENANT_NAME"),
		ProjectID:         first("OS_PROJECT_ID", "OS_TENANT_ID"),
		ProjectDomainName: getenv("OS_PROJECT_DOMAIN_NAME"),
		ProjectDomainID:   getenv("OS_PROJECT_DOMAIN_ID"),
		AppCredID:         getenv("OS_APPLICATION_CREDENTIAL_ID"),
		AppCredSecret:     getenv("OS_APPLICATION_CREDENTIAL_SECRET"),
		Region:            getenv("OS_REGION_NAME"),
		Interface:         getenv("OS_INTERFACE"),
	}
}

// Validate reports which OS_* variable is missing.
func (c Credentials) Validate() error {
	switch {
	case c.AuthURL == "":
		return errors.New("OS_AUTH_URL is not set; source your openrc first")
	case c.AppCredID != "":
		if c.AppCredSecret == "" {
			return errors.New("OS_APPLICATION_CREDENTIAL_SECRET is not set")
		}
		return nil
	case c.Username == "" || c.Password == "":
		return errors.New("OS_USERNAME and OS_PASSWORD are required")
	case c.ProjectName == "" && c.ProjectID == "":
		return errors.New("OS_PROJECT_NAME or OS_PROJECT_ID is required")
	}
	return nil
}

func (c Credentials) requestBody() map[string]any {
	if c.AppCredID != "" {
		return map[string]any{"auth": map[string]any{"identity": map[string]any{
			"methods":                []string{"application_credential"},
			"application_credential": map[string]string{"id": c.AppCredID, "secret": c.AppCredSecret},
		}}}
	}
	user := map[string]any{
		"name":     c.Username,
		"password": c.Password,
		"domain":   domain(c.UserDomainID, c.UserDomainName),
	}
	project := map[string]any{}
	if c.ProjectID != "" {
		project["id"] = c.ProjectID
	} else {
		project["name"] = c.ProjectName
		project["domain"] = domain(c.ProjectDomainID, c.ProjectDomainName)
	}
	return map[string]any{"auth": map[string]any{
		"identity": map[string]any{
			"methods":  []string{"password"},
			"password": map[string]any{"user": user},
		},
		"scope": map[string]any{"project": project},
	}}
}

func domain(id, name string) map[string]string {
	if id != "" {
		return map[string]string{"id": id}
	}
	if name == "" {
		name = "Default"
	}
	return map[string]string{"name": name}
}

// Session is a scoped token plus the catalog that came with it.
type Session struct {
	Token   string
	User    string
	Project string
	Roles   []string
	Catalog []Service
}

type Service struct {
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Endpoints []Endpoint `json:"endpoints"`
}

type Endpoint struct {
	Interface string `json:"interface"`
	Region    string `json:"region"`
	RegionID  string `json:"region_id"`
	URL       string `json:"url"`
}

// Authenticate calls POST /v3/auth/tokens. The token comes back in the
// X-Subject-Token header, not in the body.
func Authenticate(ctx context.Context, c *httpx.Client, cr Credentials) (*Session, error) {
	body, err := json.Marshal(cr.requestBody())
	if err != nil {
		return nil, err
	}
	header := http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}}
	resp, err := c.Do(ctx, http.MethodPost, tokenURL(cr.AuthURL), header, body)
	if err != nil {
		return nil, err
	}

	var out struct {
		Token struct {
			User struct {
				Name string `json:"name"`
			} `json:"user"`
			Project struct {
				Name string `json:"name"`
			} `json:"project"`
			Roles []struct {
				Name string `json:"name"`
			} `json:"roles"`
			Catalog []Service `json:"catalog"`
		} `json:"token"`
	}
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	tok := resp.Header.Get("X-Subject-Token")
	if tok == "" {
		return nil, errors.New("keystone returned no X-Subject-Token header")
	}

	s := &Session{
		Token:   tok,
		User:    out.Token.User.Name,
		Project: out.Token.Project.Name,
		Catalog: out.Token.Catalog,
	}
	for _, r := range out.Token.Roles {
		s.Roles = append(s.Roles, r.Name)
	}
	return s, nil
}

func tokenURL(authURL string) string {
	u := strings.TrimRight(authURL, "/")
	if !strings.HasSuffix(u, "/v3") {
		u += "/v3"
	}
	return u + "/auth/tokens"
}

// Endpoint picks the URL of serviceType from the catalog. iface defaults to
// "public"; an empty region matches any region.
func (s *Session) Endpoint(serviceType, iface, region string) (string, error) {
	iface = strings.TrimSuffix(iface, "URL") // accept legacy "publicURL"
	if iface == "" {
		iface = "public"
	}
	for _, svc := range s.Catalog {
		if svc.Type != serviceType {
			continue
		}
		for _, ep := range svc.Endpoints {
			if ep.Interface != iface {
				continue
			}
			if region != "" && ep.Region != region && ep.RegionID != region {
				continue
			}
			return ep.URL, nil
		}
	}
	return "", fmt.Errorf("catalog has no %s endpoint for service type %q", iface, serviceType)
}

// HasRole reports whether the token carries the named role.
func (s *Session) HasRole(name string) bool {
	for _, r := range s.Roles {
		if r == name {
			return true
		}
	}
	return false
}
