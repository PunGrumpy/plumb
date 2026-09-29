// Package update tells the user when a newer plumb release exists.
//
// The check reads one URL that returns the latest release as JSON. Both the
// GitHub releases API ({"tag_name", "html_url"}) and a plain
// {"version", "url"} document work. The result is cached for a day, so most
// runs make no request at all.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Release is the newest version the URL reports.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

type cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    Release   `json:"latest"`
}

// Checker fetches the latest release, at most once per TTL.
type Checker struct {
	URL       string
	CachePath string
	HTTP      *http.Client
	TTL       time.Duration
	Now       func() time.Time
}

// CachePath returns $XDG_CACHE_HOME/plumb/update.json, or the same under
// ~/.cache.
func CachePath(getenv func(string) string) string {
	dir := getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir = filepath.Join(getenv("HOME"), ".cache")
	}
	return filepath.Join(dir, "plumb", "update.json")
}

// Latest returns the cached release when it is fresh, and fetches it
// otherwise.
func (c Checker) Latest(ctx context.Context) (Release, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	ttl := c.TTL
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	if data, err := os.ReadFile(c.CachePath); err == nil {
		var cached cache
		if json.Unmarshal(data, &cached) == nil && now().Sub(cached.CheckedAt) < ttl {
			return cached.Latest, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Release{}, err
	}

	rel, err := c.fetch(ctx)
	if err != nil {
		return Release{}, err
	}
	data, _ := json.Marshal(cache{CheckedAt: now(), Latest: rel})
	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o700); err == nil {
		os.WriteFile(c.CachePath, data, 0o600)
	}
	return rel, nil
}

func (c Checker) fetch(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/json")
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, errors.New("update check: " + resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Release{}, err
	}
	rel := Release{Version: body.Version, URL: body.URL}
	if rel.Version == "" {
		rel = Release{Version: body.TagName, URL: body.HTMLURL}
	}
	if rel.Version == "" {
		return Release{}, errors.New("update check: no version in response")
	}
	return rel, nil
}

// Newer reports whether latest is a higher version than current. Versions
// look like v1.2.3; a suffix such as -rc1 or -dirty is ignored. Anything
// that does not parse, such as "dev", is never older.
func Newer(current, latest string) bool {
	c, ok1 := parse(current)
	l, ok2 := parse(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
