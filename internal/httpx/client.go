// Package httpx is the one HTTP layer every service client goes through, so
// that --dump-http, --record and --replay see every call plumb makes.
package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxBody = 32 << 20

// Client wraps http.Client and logs each call to Dump when it is set.
type Client struct {
	HTTP *http.Client
	Dump io.Writer
	// OnRequest, when set, is called before each request is sent.
	OnRequest func(method, url string)
}

// Response is a fully read HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// StatusError is returned for any non-2xx response.
type StatusError struct {
	Method string
	URL    string
	Code   int
	Body   string
}

func (e *StatusError) Error() string {
	body := strings.Join(strings.Fields(e.Body), " ")
	if len(body) > 200 {
		body = body[:200] + "…"
	}
	return fmt.Sprintf("%s %s: HTTP %d %s", e.Method, e.URL, e.Code, body)
}

// IsNotFound reports whether err is an HTTP 404.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

// Do sends one request and reads the whole body.
func (c *Client) Do(ctx context.Context, method, rawURL string, header http.Header, body []byte) (*Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return nil, err
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	if c.OnRequest != nil {
		c.OnRequest(method, rawURL)
	}
	start := time.Now()
	resp, err := hc.Do(req)
	if err != nil {
		c.log(method, rawURL, fmt.Sprintf("error: %v", err), start)
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	c.log(method, rawURL, fmt.Sprintf("%d, %s", resp.StatusCode, formatSize(len(data))), start)
	if err != nil {
		return nil, fmt.Errorf("%s %s: read body: %w", method, rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, &StatusError{Method: method, URL: rawURL, Code: resp.StatusCode, Body: string(data)}
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

func (c *Client) log(method, rawURL, result string, start time.Time) {
	if c.Dump == nil {
		return
	}
	elapsed := time.Since(start).Round(time.Millisecond)
	fmt.Fprintf(c.Dump, "%-4s %s → %s, %s\n", method, rawURL, result, elapsed)
}

func formatSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f KB", float64(n)/1024)
}

// Service is a Client bound to one base URL and a fixed set of headers.
type Service struct {
	Client *Client
	Base   string
	Header http.Header
}

// URL joins the base URL, path and query.
func (s *Service) URL(path string, q url.Values) string {
	u := strings.TrimRight(s.Base, "/") + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

// Get returns the raw response body.
func (s *Service) Get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	resp, err := s.Client.Do(ctx, http.MethodGet, s.URL(path, q), s.Header, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// GetJSON decodes the response body into out.
func (s *Service) GetJSON(ctx context.Context, path string, q url.Values, out any) error {
	body, err := s.Get(ctx, path, q)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s: %w", s.URL(path, q), err)
	}
	return nil
}
