package doctor

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"plumb/internal/httpx"
	"plumb/internal/keystone"
	"plumb/internal/trace"
	"plumb/internal/ui"
)

// fakeCloud answers as Keystone, the OpenStack APIs, the Config API, a
// control node and a vRouter agent, all on one port. Its Config API lists
// two virtual-routers: one at the server's own address and one at a port
// where nothing listens.
func fakeCloud(t *testing.T) (*httptest.Server, string) {
	dead := httptest.NewServer(http.NotFoundHandler())
	deadAddr := dead.Listener.Addr().String()
	dead.Close()

	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v3/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
		ep := func(typ string) string {
			return fmt.Sprintf(`{"type":%q,"endpoints":[{"interface":"public","url":%q}]}`, typ, srv.URL+"/"+typ)
		}
		w.Header().Set("X-Subject-Token", "tok")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"token":{"user":{"name":"admin"},"project":{"name":"admin"},"roles":[{"name":"admin"}],"catalog":[%s,%s,%s]}}`,
			ep("compute"), ep("network"), ep("image"))
	})
	mux.HandleFunc("GET /virtual-routers", func(w http.ResponseWriter, r *http.Request) {
		live, _ := url.Parse(srv.URL)
		fmt.Fprintf(w, `{"virtual-routers":[
			{"virtual-router":{"fq_name":["g","compute-01"],"virtual_router_ip_address":%q}},
			{"virtual-router":{"fq_name":["g","compute-02"],"virtual_router_ip_address":%q}}]}`,
			live.Host, deadAddr)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // an HTTP answer still counts as reachable
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, deadAddr
}

func TestDoctor(t *testing.T) {
	srv, deadAddr := fakeCloud(t)
	checks := Run(context.Background(), Options{
		HTTP:         &httpx.Client{HTTP: &http.Client{}},
		Creds:        keystone.Credentials{AuthURL: srv.URL, Username: "u", Password: "p", ProjectName: "admin"},
		ConfigURL:    srv.URL,
		ControlURLs:  []string{srv.URL},
		ProbeTimeout: 2 * time.Second,
		// Each virtual-router address in fakeCloud already carries its port.
		agentURL: func(addr string) string { return "http://" + addr },
	})

	want := map[string]trace.Status{
		"keystone": trace.StatusOK, "compute": trace.StatusOK, "network": trace.StatusOK, "image": trace.StatusOK,
		"opensdn-config": trace.StatusOK, "control": trace.StatusOK, "vrouter": trace.StatusWarn,
	}
	for _, c := range checks {
		if c.Status != want[c.Name] {
			t.Errorf("%s = %s (%s), want %s", c.Name, c.Status, c.Detail, want[c.Name])
		}
	}

	var out bytes.Buffer
	Render(&out, checks, ui.Palette{})
	for _, s := range []string{"1 of 2 reachable", "compute-02 " + deadAddr, "This machine can trace, with gaps at vrouter"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("report lacks %q\n%s", s, out.String())
		}
	}
}

func TestDoctorWithoutCredentials(t *testing.T) {
	checks := Run(context.Background(), Options{HTTP: &httpx.Client{}})
	if checks[0].Status != trace.StatusFail || !strings.Contains(checks[0].Hint, "plumb demo") {
		t.Errorf("keystone check = %+v", checks[0])
	}
	for _, c := range checks[1:] {
		if c.Status != trace.StatusSkip {
			t.Errorf("%s = %s, want skip", c.Name, c.Status)
		}
	}
}
