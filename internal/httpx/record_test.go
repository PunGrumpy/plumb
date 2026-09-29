package httpx

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestKey(t *testing.T) {
	tests := []struct{ url, want string }{
		{"http://demo.lab/compute/v2.1/servers/abc", "GET_demo.lab_compute_v2.1_servers_abc.http"},
		{"http://10.0.0.1:8083/Snh_ShowRouteReq?prefix=10.0.1.5%2F32&routing_table=a%3Ab.inet.0",
			"GET_10.0.0.1_8083_Snh_ShowRouteReq_prefix_10.0.1.5_32_routing_table_a_b.inet.0.http"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.url, nil)
		if got := Key(req); got != tt.want {
			t.Errorf("Key(%s) = %s, want %s", tt.url, got, tt.want)
		}
	}
}

func TestRecordThenReplay(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Subject-Token", "secret")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	rec := &Client{HTTP: &http.Client{Transport: &Recorder{Dir: dir, Next: http.DefaultTransport}}}
	if _, err := rec.Do(context.Background(), http.MethodPost, srv.URL+"/v3/auth/tokens", nil, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	srv.Close()

	files, _ := filepath.Glob(filepath.Join(dir, "*.http"))
	if len(files) != 1 {
		t.Fatalf("recorded %d files, want 1", len(files))
	}
	raw, _ := os.ReadFile(files[0])
	if string(raw) == "" || bytes.Contains(raw, []byte("secret")) {
		t.Fatalf("recording leaks the token or is empty:\n%s", raw)
	}

	rep := &Client{HTTP: &http.Client{Transport: &Replayer{Read: DirReader(dir)}}}
	resp, err := rep.Do(context.Background(), http.MethodPost, srv.URL+"/v3/auth/tokens", nil, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusCreated || string(resp.Body) != `{"ok":true}` {
		t.Fatalf("replayed %d %q", resp.Status, resp.Body)
	}
	if resp.Header.Get("X-Subject-Token") != "recorded-token" {
		t.Fatalf("token header = %q", resp.Header.Get("X-Subject-Token"))
	}
}
