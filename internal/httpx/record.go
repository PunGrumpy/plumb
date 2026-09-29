package httpx

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Recordings are plain HTTP responses on disk:
//
//	HTTP/1.1 200 OK
//	Content-Type: application/json
//
//	{"server": …}
//
// so they can be written by hand and read back with http.ReadResponse.

// keptHeaders are the only response headers a recording keeps.
var keptHeaders = []string{"Content-Type", "X-Subject-Token"}

// Key returns the recording file name for a request.
func Key(r *http.Request) string {
	s := r.Method + " " + r.URL.Host + r.URL.Path
	if r.URL.RawQuery != "" {
		q, err := url.QueryUnescape(r.URL.RawQuery)
		if err != nil {
			q = r.URL.RawQuery
		}
		s += "?" + q
	}

	var b strings.Builder
	under := false
	for _, c := range s {
		ok := c == '.' || c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		switch {
		case ok:
			b.WriteRune(c)
			under = false
		case !under:
			b.WriteByte('_')
			under = true
		}
	}
	name := strings.TrimRight(b.String(), "_")
	if len(name) > 150 {
		h := fnv.New64a()
		h.Write([]byte(name))
		name = fmt.Sprintf("%s_%x", name[:150], h.Sum64())
	}
	return name + ".http"
}

// Recorder saves every response it forwards into Dir.
type Recorder struct {
	Dir  string
	Next http.RoundTripper
}

func (t *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.Next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))

	var b bytes.Buffer
	fmt.Fprintf(&b, "HTTP/1.1 %s\n", resp.Status)
	for _, k := range keptHeaders {
		v := resp.Header.Get(k)
		if v == "" {
			continue
		}
		if k == "X-Subject-Token" {
			v = "recorded-token"
		}
		fmt.Fprintf(&b, "%s: %s\n", k, v)
	}
	b.WriteString("\n")
	b.Write(body)

	if err := os.MkdirAll(t.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("record: %w", err)
	}
	if err := os.WriteFile(filepath.Join(t.Dir, Key(req)), b.Bytes(), 0o600); err != nil {
		return nil, fmt.Errorf("record: %w", err)
	}
	return resp, nil
}

// Replayer answers every request from recordings and never touches the
// network. Read returns the recording for a file name from Key.
type Replayer struct {
	Read func(name string) ([]byte, error)
}

// DirReader reads recordings from a directory on disk.
func DirReader(dir string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(dir, name))
	}
}

func (t *Replayer) RoundTrip(req *http.Request) (*http.Response, error) {
	name := Key(req)
	data, err := t.Read(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("replay: no recording %s", name)
	}
	if err != nil {
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), req)
}
