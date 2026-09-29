package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestNewer(t *testing.T) {
	tests := []struct {
		cur, latest string
		want        bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.9.9", "v1.0.0", true},
		{"v1.2.3", "v1.2.3", false},
		{"v1.3.0", "v1.2.9", false},
		{"v1.2.3-dirty", "v1.2.4", true},
		{"dev", "v9.9.9", false},
		{"v1.2.3", "nightly", false},
	}
	for _, tt := range tests {
		if got := Newer(tt.cur, tt.latest); got != tt.want {
			t.Errorf("Newer(%q, %q) = %v", tt.cur, tt.latest, got)
		}
	}
}

func TestLatestCachesForTTL(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write([]byte(`{"tag_name":"v1.4.0","html_url":"https://example.test/r/v1.4.0"}`))
	}))
	defer srv.Close()

	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	c := Checker{URL: srv.URL, CachePath: filepath.Join(t.TempDir(), "u.json"), Now: func() time.Time { return now }}
	for range 3 {
		rel, err := c.Latest(context.Background())
		if err != nil || rel.Version != "v1.4.0" || rel.URL != "https://example.test/r/v1.4.0" {
			t.Fatalf("Latest = %+v, %v", rel, err)
		}
	}
	if calls != 1 {
		t.Errorf("fetched %d times within the TTL, want 1", calls)
	}
	now = now.Add(25 * time.Hour)
	c.Latest(context.Background())
	if calls != 2 {
		t.Errorf("fetched %d times after the TTL, want 2", calls)
	}
}
