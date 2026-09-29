package trace

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PunGrumpy/plumb/apps/cli/internal/httpx"
)

func ptr(n int) *int { return &n }

func TestAllows(t *testing.T) {
	icmp := &Path{Protocol: "icmp"}
	ssh := &Path{Protocol: "tcp", Port: 22}
	peer := "10.0.1.5"
	tests := []struct {
		name string
		p    *Path
		r    Rule
		want bool
	}{
		{"any protocol any peer", ssh, Rule{Direction: "ingress", EtherType: "IPv4"}, true},
		{"tcp port in range", ssh, Rule{EtherType: "IPv4", Protocol: "tcp", PortMin: ptr(20), PortMax: ptr(25)}, true},
		{"tcp port outside range", ssh, Rule{EtherType: "IPv4", Protocol: "tcp", PortMin: ptr(80), PortMax: ptr(80)}, false},
		{"protocol by number", ssh, Rule{EtherType: "IPv4", Protocol: "6", PortMin: ptr(22), PortMax: ptr(22)}, true},
		{"udp rule, tcp traffic", ssh, Rule{EtherType: "IPv4", Protocol: "udp"}, false},
		{"icmp any type", icmp, Rule{EtherType: "IPv4", Protocol: "icmp"}, true},
		{"icmp echo request", icmp, Rule{EtherType: "IPv4", Protocol: "icmp", PortMin: ptr(8)}, true},
		{"icmp echo reply only", icmp, Rule{EtherType: "IPv4", Protocol: "icmp", PortMin: ptr(0)}, false},
		{"IPv6 rule", icmp, Rule{EtherType: "IPv6"}, false},
		{"prefix contains peer", ssh, Rule{EtherType: "IPv4", RemoteIPPrefix: "10.0.1.0/24"}, true},
		{"prefix excludes peer", ssh, Rule{EtherType: "IPv4", RemoteIPPrefix: "10.0.2.0/24"}, false},
		{"peer in remote group", ssh, Rule{EtherType: "IPv4", RemoteGroupID: "sg-web"}, true},
		{"peer not in remote group", ssh, Rule{EtherType: "IPv4", RemoteGroupID: "sg-db"}, false},
	}
	for _, tt := range tests {
		if got := tt.p.allows(tt.r, peer, []string{"sg-web"}); got != tt.want {
			t.Errorf("%s: allows = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// vm builds a traced VM with one port, for path checks without a cloud.
func vm(name, ip, cidr, network string, routers []Router, sgs ...SecurityGroup) *Trace {
	return &Trace{
		Query:  name,
		Server: &Server{Name: name},
		Ports: []*Port{{
			ID:  name + "-port",
			IPs: []string{ip},
			Neutron: &NeutronPort{
				Status:         "ACTIVE",
				VIFType:        "vrouter",
				PortSecurity:   true,
				Network:        &Network{ID: network, Name: network, Routers: routers},
				Subnets:        []Subnet{{CIDR: cidr}},
				SecurityGroups: sgs,
			},
		}},
	}
}

var (
	allowAll = SecurityGroup{ID: "sg-open", Name: "open", Rules: []Rule{
		{Direction: "egress", EtherType: "IPv4"}, {Direction: "ingress", EtherType: "IPv4"},
	}}
	egressOnly = SecurityGroup{ID: "sg-closed", Name: "closed", Rules: []Rule{{Direction: "egress", EtherType: "IPv4"}}}
)

func status(p *Path) map[string]Status {
	out := map[string]Status{}
	for _, c := range p.Checks {
		out[c.Name] = c.Status
	}
	return out
}

func TestPathNeutronChecks(t *testing.T) {
	r1 := []Router{{ID: "r1", Name: "r1"}}
	tests := []struct {
		name     string
		from, to *Trace
		want     map[string]Status
		code     string
	}{
		{"same subnet", vm("a", "10.0.1.5", "10.0.1.0/24", "net1", nil, allowAll), vm("b", "10.0.1.6", "10.0.1.0/24", "net1", nil, allowAll),
			map[string]Status{"network": StatusOK, "egress": StatusOK, "ingress": StatusOK}, ""},
		{"shared router", vm("a", "10.0.1.5", "10.0.1.0/24", "net1", r1, allowAll), vm("b", "10.0.2.6", "10.0.2.0/24", "net2", r1, allowAll),
			map[string]Status{"network": StatusOK}, ""},
		{"no router", vm("a", "10.0.1.5", "10.0.1.0/24", "net1", nil, allowAll), vm("b", "10.0.2.6", "10.0.2.0/24", "net2", nil, allowAll),
			map[string]Status{"network": StatusFail}, "path-no-router"},
		{"ingress blocked", vm("a", "10.0.1.5", "10.0.1.0/24", "net1", nil, allowAll), vm("b", "10.0.1.6", "10.0.1.0/24", "net1", nil, egressOnly),
			map[string]Status{"egress": StatusOK, "ingress": StatusFail}, "sg-ingress-blocked"},
	}
	for _, tt := range tests {
		p := &Path{From: tt.from, To: tt.to, Protocol: "tcp", Port: 22}
		p.check(context.Background(), &httpx.Client{})
		got := status(p)
		for name, want := range tt.want {
			if got[name] != want {
				t.Errorf("%s: %s = %s, want %s (%+v)", tt.name, name, got[name], want, p.Checks)
			}
		}
		if tt.code != "" {
			found := false
			for _, c := range p.Checks {
				found = found || c.Code == tt.code
			}
			if !found {
				t.Errorf("%s: no check with code %s", tt.name, tt.code)
			}
		}
		if got["route"] != StatusSkip {
			t.Errorf("%s: route = %s, want skip without an agent", tt.name, got["route"])
		}
	}
}

func TestIngressHintIsACommand(t *testing.T) {
	p := &Path{From: vm("a", "10.0.1.5", "10.0.1.0/24", "n", nil, allowAll), To: vm("b", "10.0.1.6", "10.0.1.0/24", "n", nil, egressOnly), Protocol: "tcp", Port: 5432}
	p.check(context.Background(), &httpx.Client{})
	want := "openstack security group rule create --ingress --protocol tcp --dst-port 5432 --remote-ip 10.0.1.5/32 closed"
	for _, c := range p.Checks {
		if c.Name == "ingress" && !strings.Contains(c.Hint, want) {
			t.Errorf("hint = %q", c.Hint)
		}
	}
}

// agentServer answers Inet4UcRouteReq with one path, like a vRouter agent.
func agentServer(t *testing.T, nhType, itf, dip, label string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<Inet4UcRouteResp><route_list><list><RouteUcSandeshData>
<src_ip>%s</src_ip><src_plen>32</src_plen><path_list><list><PathSandeshData>
<nh><NhSandeshData><type>%s</type><itf>%s</itf><dip>%s</dip><tunnel_type>MPLSoUDP</tunnel_type></NhSandeshData></nh>
<label>%s</label></PathSandeshData></list></path_list></RouteUcSandeshData></list></route_list></Inet4UcRouteResp>`,
			r.URL.Query().Get("src_ip"), nhType, itf, dip, label)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func onOpenSDN(t *Trace, computeIP, agentURL, itf, label string) *Trace {
	t.Compute = &Compute{Name: "cmp-" + computeIP, IP: computeIP, AgentURL: agentURL}
	t.Ports[0].VRouter = &VRouterView{VRFIndex: 3, Interface: &AgentInterface{Name: itf, Active: true, VRF: "d:p:net1:net1", Label: label}}
	return t
}

func TestPathDatapath(t *testing.T) {
	tests := []struct {
		name                     string
		nhType, itf, dip, label  string
		toCompute, toItf, toLabl string
		want                     Status
		code                     string
	}{
		{"remote, label matches", "tunnel", "", "10.10.0.22", "31", "10.10.0.22", "tap-b", "31", StatusOK, ""},
		{"remote, stale label", "tunnel", "", "10.10.0.22", "25", "10.10.0.22", "tap-b", "31", StatusFail, "label-mismatch"},
		{"remote, wrong compute", "tunnel", "", "10.10.0.23", "31", "10.10.0.22", "tap-b", "31", StatusFail, "path-wrong-next-hop"},
		{"same compute", "interface", "tap-b", "", "31", "10.10.0.21", "tap-b", "31", StatusOK, ""},
	}
	for _, tt := range tests {
		url := agentServer(t, tt.nhType, tt.itf, tt.dip, tt.label)
		from := onOpenSDN(vm("a", "10.0.1.5", "10.0.1.0/24", "net1", nil, allowAll), "10.10.0.21", url, "tap-a", "25")
		to := onOpenSDN(vm("b", "10.0.1.6", "10.0.1.0/24", "net1", nil, allowAll), tt.toCompute, "", tt.toItf, tt.toLabl)
		p := &Path{From: from, To: to, Protocol: "icmp"}
		p.check(context.Background(), &httpx.Client{HTTP: http.DefaultClient})
		got := status(p)
		if got["route"] != StatusOK || got["next-hop"] != tt.want {
			t.Errorf("%s: route %s, next-hop %s, want ok and %s\n%+v", tt.name, got["route"], got["next-hop"], tt.want, p.Checks)
		}
		if tt.code != "" && p.Checks[len(p.Checks)-1].Code != tt.code {
			t.Errorf("%s: code %s, want %s", tt.name, p.Checks[len(p.Checks)-1].Code, tt.code)
		}
	}
}
