// Package profile stores the OpenSDN URLs that `plumb link` saves for each
// cloud, so they do not need to be passed on every command.
//
// A cloud is identified by its Keystone URL (OS_AUTH_URL). The file is
// $XDG_CONFIG_HOME/plumb/config.json, or ~/.config/plumb/config.json.
package profile

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Link is what plumb remembers about one cloud.
type Link struct {
	ConfigURL   string   `json:"config_url"`
	ControlURLs []string `json:"control_urls,omitempty"`
}

// File is the whole config file.
type File struct {
	Clouds map[string]Link `json:"clouds"`
}

// Path returns the config file path. PLUMB_CONFIG overrides it.
func Path(getenv func(string) string) string {
	if p := getenv("PLUMB_CONFIG"); p != "" {
		return p
	}
	dir := getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "plumb", "config.json")
}

// Key normalises an auth URL so that http://h/identity and
// http://h/identity/v3/ name the same cloud.
func Key(authURL string) string {
	u := strings.TrimRight(authURL, "/")
	return strings.TrimSuffix(u, "/v3")
}

// Load reads the file at path. A missing file is an empty config.
func Load(path string) (*File, error) {
	f := &File{Clouds: map[string]Link{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, f); err != nil {
		return nil, err
	}
	if f.Clouds == nil {
		f.Clouds = map[string]Link{}
	}
	return f, nil
}

// Save writes the file readable only by the owner, because it lists
// internal addresses.
func (f *File) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Lookup returns the link for authURL.
func (f *File) Lookup(authURL string) (Link, bool) {
	l, ok := f.Clouds[Key(authURL)]
	return l, ok
}
