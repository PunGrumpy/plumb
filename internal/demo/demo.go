// Package demo ships a recorded lab inside the binary, so `plumb demo`
// works without a cloud, credentials or a checkout of this repo.
//
// The lab is one VM, web-01, on compute-02 behind two control nodes. Each
// scenario patches the recordings to break one layer, which shows what
// plumb reports when that layer fails.
package demo

import (
	"embed"
	"io/fs"
	"net"
	"regexp"
	"strings"
	"syscall"
)

//go:embed lab/*.http
var lab embed.FS

// Values that match the recordings in lab/.
const (
	VM        = "3f2a8c1e-5b7d-4e2a-9c61-0d4b8e7f1a23"
	VMName    = "web-01"
	AuthURL   = "http://demo.lab/identity"
	ConfigURL = "http://config.lab:8082"
)

type Scenario struct {
	Name        string
	Description string
	// NoOpenSDN runs without a Config API URL, as on DevStack with OVN.
	NoOpenSDN bool
	patch     func(name string, data []byte, err error) ([]byte, error)
}

// Read returns the recording for a file name from httpx.Key.
func (s Scenario) Read(name string) ([]byte, error) {
	data, err := lab.ReadFile("lab/" + name)
	if s.patch != nil {
		return s.patch(name, data, err)
	}
	return data, err
}

var showRoute = regexp.MustCompile(`(?s)\s*<ShowRoute>.*?</ShowRoute>`)

// Scenarios lists every scenario; the first is the default.
var Scenarios = []Scenario{
	{
		Name:        "healthy",
		Description: "every layer agrees; this is a working VM",
	},
	{
		Name:        "devstack",
		Description: "no OpenSDN, as on DevStack with OVN; the last 3 stages skip",
		NoOpenSDN:   true,
		patch: func(name string, data []byte, err error) ([]byte, error) {
			if strings.Contains(name, "_ports_device_id_") {
				return []byte(strings.Replace(string(data), `"binding:vif_type": "vrouter"`, `"binding:vif_type": "ovs"`, 1)), err
			}
			return data, err
		},
	},
	{
		Name:        "vmi-missing",
		Description: "the Neutron port has no VMI in OpenSDN; the two are out of sync",
		patch: func(name string, data []byte, err error) ([]byte, error) {
			if strings.Contains(name, "virtual-machine-interface_") {
				return []byte("HTTP/1.1 404 Not Found\nContent-Type: text/plain\n\n404 Not Found\n"), nil
			}
			return data, err
		},
	},
	{
		Name:        "missing-route",
		Description: "control-02 has no route for the VM",
		patch: func(name string, data []byte, err error) ([]byte, error) {
			if strings.HasPrefix(name, "GET_10.10.0.12_8083_Snh_ShowRouteReq") {
				return showRoute.ReplaceAll(data, nil), err
			}
			return data, err
		},
	},
	{
		Name:        "label-mismatch",
		Description: "the agent assigned a new label that the control nodes do not advertise",
		patch: func(name string, data []byte, err error) ([]byte, error) {
			if strings.HasPrefix(name, "GET_10.10.0.21_8085_Snh_ItfReq") {
				return []byte(strings.Replace(string(data), `<label type="i32">25</label>`, `<label type="i32">31</label>`, 1)), err
			}
			return data, err
		},
	},
	{
		Name:        "agent-down",
		Description: "nothing answers on the compute's introspect port 8085",
		patch: func(name string, data []byte, err error) ([]byte, error) {
			if strings.HasPrefix(name, "GET_10.10.0.21_8085_") {
				return nil, &net.OpError{Op: "dial", Net: "tcp", Addr: fakeAddr("10.10.0.21:8085"), Err: syscall.ECONNREFUSED}
			}
			return data, err
		},
	},
}

// Find returns the scenario called name.
func Find(name string) (Scenario, bool) {
	for _, s := range Scenarios {
		if s.Name == name {
			return s, true
		}
	}
	return Scenario{}, false
}

// Files lists the recordings in the lab, for tests and --list.
func Files() ([]string, error) {
	return fs.Glob(lab, "lab/*.http")
}

type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }
