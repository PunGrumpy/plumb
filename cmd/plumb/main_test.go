package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"plumb/internal/demo"
	"plumb/internal/trace"
	"plumb/internal/ui"
	"plumb/internal/update"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

func runCLI(t *testing.T, vars map[string]string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = run(args, env{stdout: &out, stderr: &errOut, getenv: func(k string) string { return vars[k] }})
	return out.String(), errOut.String(), code
}

func TestDemoGolden(t *testing.T) {
	out, stderr, code := runCLI(t, nil, "demo", "--no-timings")
	if code != 0 {
		t.Fatalf("exit %d\n%s%s", code, out, stderr)
	}
	golden := "testdata/demo.golden"
	if *updateGolden {
		os.WriteFile(golden, []byte(out), 0o644)
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if out != string(want) {
		t.Errorf("output differs from %s (run make golden)\n%s", golden, out)
	}
}

// Each scenario breaks one layer. The verdict at the end of the output must
// name that layer and the code `plumb explain` knows.
func TestDemoScenarios(t *testing.T) {
	tests := []struct {
		scenario string
		code     int
		want     []string
	}{
		{"devstack", 0, []string{
			"✓ Traced web-01 through 3 of 6 stages",
			"Skipped opensdn-config: ports use vif_type ovs, so this cloud does not run OpenSDN",
		}},
		{"vmi-missing", 1, []string{
			"✗ Trace of web-01 stopped at opensdn-config",
			"plumb explain vmi-missing",
		}},
		{"missing-route", 0, []string{
			"control-02  vn1:vn1.inet.0  10.0.1.5/32 ✗ missing",
			"! Traced web-01 through 6 of 6 stages, 1 issue",
			"plumb explain route-missing",
		}},
		{"label-mismatch", 0, []string{
			"control-01 advertises 10.0.1.5/32 with label 25, agent assigned 31",
			"plumb explain label-mismatch",
			"1 more under Steps",
		}},
		{"agent-down", 1, []string{
			"✗ Trace of web-01 stopped at vrouter",
			"connection refused",
			"plumb explain unreachable",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			out, _, code := runCLI(t, nil, "demo", tt.scenario, "--no-timings")
			if code != tt.code {
				t.Errorf("exit %d, want %d", code, tt.code)
			}
			for _, s := range tt.want {
				if !strings.Contains(out, s) {
					t.Errorf("output lacks %q\n%s", s, out)
				}
			}
		})
	}
}

func TestEveryScenarioIsTested(t *testing.T) {
	for _, s := range demo.Scenarios[1:] {
		out, _, _ := runCLI(t, nil, "demo", s.Name, "--no-timings")
		if strings.Contains(out, "✓ Traced web-01 through 6 of 6") {
			t.Errorf("scenario %s breaks nothing", s.Name)
		}
	}
}

func TestDemoJSONCarriesIssueCodes(t *testing.T) {
	out, _, _ := runCLI(t, nil, "demo", "missing-route", "--json")
	var tr trace.Trace
	if err := json.Unmarshal([]byte(out), &tr); err != nil {
		t.Fatal(err)
	}
	for _, s := range tr.Steps {
		if s.Name == "control" {
			if len(s.Warnings) != 1 || s.Warnings[0].Code != "route-missing" || s.Warnings[0].Hint == "" {
				t.Errorf("control warnings = %+v", s.Warnings)
			}
			return
		}
	}
	t.Error("no control step")
}

func TestReplayDirectory(t *testing.T) {
	dir := t.TempDir()
	files, _ := demo.Files()
	for _, f := range files {
		b, _ := demo.Scenarios[0].Read(filepath.Base(f))
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
	}
	vars := map[string]string{"OS_AUTH_URL": demo.AuthURL, "OPENSDN_CONFIG_URL": demo.ConfigURL}
	out, _, code := runCLI(t, vars, demo.VM, "--replay", dir, "--no-timings")
	if code != 0 || !strings.Contains(out, "✓ Traced web-01 through 6 of 6 stages") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestErrorsSayWhatToDo(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"web-01"}, "or try the offline lab with `plumb demo`"},
		{[]string{"dmeo"}, "Did you mean `plumb demo`?"},
		{[]string{"demo", "missing-rout"}, "Did you mean `plumb demo missing-route`?"},
		{[]string{"explain", "route-mising"}, "Did you mean `plumb explain route-missing`?"},
		{[]string{"a", "b"}, "Run `plumb demo`"},
	}
	for _, tt := range tests {
		_, stderr, code := runCLI(t, nil, tt.args...)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2", tt.args, code)
		}
		if !strings.Contains(stderr, "Error:") || !strings.Contains(stderr, tt.want) {
			t.Errorf("%v: stderr lacks %q\n%s", tt.args, tt.want, stderr)
		}
	}
}

func TestExplain(t *testing.T) {
	out, _, code := runCLI(t, nil, "explain", "route-missing")
	if code != 0 || !strings.Contains(out, "Next:") || !strings.Contains(out, "#route-missing") {
		t.Errorf("exit %d\n%s", code, out)
	}
	list, _, _ := runCLI(t, nil, "explain")
	for _, c := range trace.Codes() {
		if !strings.Contains(list, c) {
			t.Errorf("explain list lacks %s", c)
		}
	}
}

func TestHelpAndVersion(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"--help"}} {
		out, _, code := runCLI(t, nil, args...)
		if code != 0 || !strings.Contains(out, "plumb demo") {
			t.Errorf("%v: exit %d\n%s", args, code, out)
		}
	}
	out, _, _ := runCLI(t, nil, "version")
	if !strings.HasPrefix(out, "plumb ") {
		t.Errorf("version = %q", out)
	}
}

// Every code `plumb explain` knows must have a heading in the
// troubleshooting page, because explain links to it.
func TestEveryCodeIsDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../docs/troubleshooting.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range trace.Codes() {
		if !strings.Contains(string(doc), "### `"+c+"`") {
			t.Errorf("docs/troubleshooting.md has no heading for %s", c)
		}
	}
}

func TestLinkThenTraceUsesIt(t *testing.T) {
	cfg := httptest.NewServer(http.NotFoundHandler()) // any HTTP answer proves it listens
	defer cfg.Close()
	vars := map[string]string{"OS_AUTH_URL": demo.AuthURL + "/v3/", "PLUMB_CONFIG": filepath.Join(t.TempDir(), "config.json")}

	out, stderr, code := runCLI(t, vars, "link", "--config-url", cfg.URL)
	if code != 0 || !strings.Contains(out, "✓ Linked "+demo.AuthURL+" to OpenSDN at "+cfg.URL) {
		t.Fatalf("link: exit %d\n%s%s", code, out, stderr)
	}
	out, _, _ = runCLI(t, vars, "link")
	if !strings.Contains(out, "is linked to OpenSDN at "+cfg.URL) {
		t.Errorf("link status:\n%s", out)
	}

	// Relink to the demo lab's Config API; the trace must pick it up
	// without --config-url or OPENSDN_CONFIG_URL.
	runCLI(t, vars, "link", "--config-url", demo.ConfigURL, "--force")
	dir := t.TempDir()
	files, _ := demo.Files()
	for _, f := range files {
		b, _ := demo.Scenarios[0].Read(filepath.Base(f))
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
	}
	out, _, code = runCLI(t, vars, "trace", demo.VM, "--replay", dir, "--no-timings")
	if code != 0 || !strings.Contains(out, "through 6 of 6 stages") {
		t.Errorf("trace with link: exit %d\n%s", code, out)
	}

	out, _, _ = runCLI(t, vars, "link", "--remove")
	if !strings.Contains(out, "Removed the link") {
		t.Errorf("remove:\n%s", out)
	}
}

func TestLinkRefusesADeadURL(t *testing.T) {
	vars := map[string]string{"OS_AUTH_URL": demo.AuthURL, "PLUMB_CONFIG": filepath.Join(t.TempDir(), "c.json")}
	_, stderr, code := runCLI(t, vars, "link", "--config-url", "http://127.0.0.1:1")
	if code != 1 || !strings.Contains(stderr, "add --force") {
		t.Errorf("exit %d\n%s", code, stderr)
	}
}

func TestWhoami(t *testing.T) {
	ks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Subject-Token", "tok")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"token":{"user":{"name":"alice"},"project":{"name":"lab"},"roles":[{"name":"member"}]}}`))
	}))
	defer ks.Close()
	vars := map[string]string{"OS_AUTH_URL": ks.URL, "OS_USERNAME": "alice", "OS_PASSWORD": "x", "OS_PROJECT_NAME": "lab",
		"PLUMB_CONFIG": filepath.Join(t.TempDir(), "c.json")}
	out, _, code := runCLI(t, vars, "whoami")
	if code != 0 || !strings.Contains(out, "alice in project lab") || !strings.Contains(out, "plumb link --config-url") {
		t.Errorf("exit %d\n%s", code, out)
	}
	vars["OPENSDN_CONFIG_URL"] = "http://cfg:8082"
	out, _, _ = runCLI(t, vars, "whoami")
	if !strings.Contains(out, "http://cfg:8082, from OPENSDN_CONFIG_URL") {
		t.Errorf("source not shown\n%s", out)
	}
}

func TestUpdateNotice(t *testing.T) {
	rel := update.Release{Version: "v0.3.0", URL: "https://example.test/releases/v0.3.0"}
	got := updateNotice("v0.2.1", rel, "", ui.Palette{})
	for _, s := range []string{"Update available: plumb v0.2.1 → v0.3.0", "Get  https://example.test/releases/v0.3.0", "PLUMB_NO_UPDATE_CHECK=1"} {
		if !strings.Contains(got, s) {
			t.Errorf("notice lacks %q\n%s", s, got)
		}
	}
	if got := updateNotice("v0.2.1", rel, "brew upgrade plumb", ui.Palette{}); !strings.Contains(got, "Run   brew upgrade plumb") {
		t.Errorf("install hint missing\n%s", got)
	}
}

// An admin tracing a VM by name from another project: the project-scoped
// lookup finds nothing, so plumb searches every project.
func TestAdminFindsNameInAnotherProject(t *testing.T) {
	dir := t.TempDir()
	files, _ := demo.Files()
	for _, f := range files {
		b, _ := demo.Scenarios[0].Read(filepath.Base(f))
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
	}
	byName := "GET_demo.lab_compute_v2.1_servers_name_web-01.http"
	all, _ := os.ReadFile(filepath.Join(dir, byName))
	os.WriteFile(filepath.Join(dir, "GET_demo.lab_compute_v2.1_servers_all_tenants_1_name_web-01.http"), all, 0o600)
	os.WriteFile(filepath.Join(dir, byName), []byte("HTTP/1.1 200 OK\nContent-Type: application/json\n\n{\"servers\": []}\n"), 0o600)

	vars := map[string]string{"OS_AUTH_URL": demo.AuthURL, "OPENSDN_CONFIG_URL": demo.ConfigURL}
	out, _, code := runCLI(t, vars, "trace", demo.VMName, "--replay", dir, "--no-timings")
	if code != 0 || !strings.Contains(out, "through 6 of 6 stages") {
		t.Errorf("exit %d\n%s", code, out)
	}
}

func TestTraceByIP(t *testing.T) {
	dir := t.TempDir()
	files, _ := demo.Files()
	for _, f := range files {
		b, _ := demo.Scenarios[0].Read(filepath.Base(f))
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
	}
	vars := map[string]string{"OS_AUTH_URL": demo.AuthURL, "OPENSDN_CONFIG_URL": demo.ConfigURL}
	for _, ip := range []string{"10.0.1.5", "203.0.113.10"} {
		out, _, code := runCLI(t, vars, ip, "--replay", dir, "--no-timings")
		if code != 0 || !strings.Contains(out, "VM web-01 ("+demo.VM+")") || !strings.Contains(out, "through 6 of 6 stages") {
			t.Errorf("%s: exit %d\n%s", ip, code, out)
		}
	}
	out, _, code := runCLI(t, vars, "10.0.1.1", "--replay", dir, "--no-timings")
	if code != 1 || !strings.Contains(out, "owned by network:router_interface, not a VM") || !strings.Contains(out, "plumb explain ip-not-vm") {
		t.Errorf("router IP: exit %d\n%s", code, out)
	}
}

// Both addresses belong to web-01 in the demo lab, so every check runs,
// including the OpenSDN route that ends on the VM's own tap.
func TestPathCommand(t *testing.T) {
	dir := t.TempDir()
	files, _ := demo.Files()
	for _, f := range files {
		b, _ := demo.Scenarios[0].Read(filepath.Base(f))
		os.WriteFile(filepath.Join(dir, filepath.Base(f)), b, 0o600)
	}
	vars := map[string]string{"OS_AUTH_URL": demo.AuthURL, "OPENSDN_CONFIG_URL": demo.ConfigURL}
	out, _, code := runCLI(t, vars, "path", "10.0.1.5", "203.0.113.10", "--port", "22", "--replay", dir)
	for _, s := range []string{"Path web-01 → web-01, tcp 22", "✓ ingress", "✓ route", "✓ next-hop  delivered to tap9c1e4d2b-7a", "✓ web-01 can reach web-01 over tcp 22"} {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q\n%s", s, out)
		}
	}
	if code != 0 {
		t.Errorf("exit %d", code)
	}

	_, stderr, code := runCLI(t, vars, "path", "a", "b", "--proto", "tcp")
	if code != 2 || !strings.Contains(stderr, "--port between 1 and 65535 is required") {
		t.Errorf("missing port: exit %d\n%s", code, stderr)
	}
}
