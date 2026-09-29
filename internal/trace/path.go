package trace

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	"plumb/internal/httpx"
	"plumb/internal/opensdn/agent"
)

// PathOptions names the traffic to check. Protocol is icmp, tcp or udp;
// ICMP means an echo request, as ping sends.
type PathOptions struct {
	Protocol string
	Port     int
}

// Path is the answer to "can From reach To": both traces, then one check
// per hop in the order a packet meets them.
type Path struct {
	From     *Trace  `json:"from"`
	To       *Trace  `json:"to"`
	SrcIP    string  `json:"src_ip,omitempty"`
	DstIP    string  `json:"dst_ip,omitempty"`
	Protocol string  `json:"protocol"`
	Port     int     `json:"port,omitempty"`
	Checks   []Check `json:"checks"`
}

// Check is one hop of the path. Hint is specific to this path, often the
// command that fixes it; Code is the key for `plumb explain`.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Code   string `json:"code,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

// Failed reports whether any check failed.
func (p *Path) Failed() bool {
	for _, c := range p.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// TracePath traces both VMs and then checks the path between them: ports,
// L3 connectivity, both security groups and, on OpenSDN, the route and
// next hop in the source VRF.
func TracePath(ctx context.Context, from, to string, opts Options, po PathOptions) *Path {
	stage := opts.OnStage
	prefixed := func(vm string) Options {
		o := opts
		if stage != nil {
			o.OnStage = func(s string) { stage(vm + " " + s) }
		}
		return o
	}
	p := &Path{
		From:     Run(ctx, from, prefixed(from)),
		To:       Run(ctx, to, prefixed(to)),
		Protocol: po.Protocol,
		Port:     po.Port,
	}
	if stage != nil {
		stage("path")
	}
	p.check(ctx, opts.HTTP)
	return p
}

// Name is how the output refers to a traced VM.
func Name(t *Trace) string {
	if t.Server != nil && t.Server.Name != "" {
		return t.Server.Name
	}
	return t.Query
}

func (p *Path) add(c Check) { p.Checks = append(p.Checks, c) }

func (p *Path) check(ctx context.Context, hc *httpx.Client) {
	a, b := firstPort(p.From), firstPort(p.To)
	for _, side := range []struct {
		t    *Trace
		port *Port
	}{{p.From, a}, {p.To, b}} {
		if side.port == nil {
			p.add(Check{Name: "resolve", Status: StatusFail, Detail: fmt.Sprintf("no Neutron port for %s: %s", side.t.Query, stopReason(side.t))})
			return
		}
	}
	p.SrcIP, p.DstIP = firstIPv4(a), firstIPv4(b)
	if p.SrcIP == "" || p.DstIP == "" {
		p.add(Check{Name: "resolve", Status: StatusFail, Detail: "both VMs need an IPv4 address; plumb path does not check IPv6 yet"})
		return
	}
	p.add(Check{Name: "resolve", Status: StatusOK, Detail: fmt.Sprintf("%s %s → %s %s", Name(p.From), p.SrcIP, Name(p.To), p.DstIP)})
	p.add(p.ports(a, b))
	p.add(p.network(a, b))
	p.add(p.securityGroup("egress", p.From, a, b, p.DstIP))
	p.add(p.securityGroup("ingress", p.To, b, a, p.SrcIP))
	p.Checks = append(p.Checks, p.datapath(ctx, hc, a, b)...)
}

func firstPort(t *Trace) *Port {
	for _, p := range t.Ports {
		if p.Neutron != nil {
			return p
		}
	}
	return nil
}

func firstIPv4(p *Port) string {
	for _, ip := range p.IPs {
		if parsed := net.ParseIP(ip); parsed != nil && parsed.To4() != nil {
			return ip
		}
	}
	return ""
}

// stopReason explains why a trace found no port: the first failed stage.
func stopReason(t *Trace) string {
	for _, s := range t.Steps {
		if s.Status == StatusFail {
			return s.Name + " failed: " + s.Error
		}
	}
	return "the VM has no port"
}

func (p *Path) ports(a, b *Port) Check {
	for _, side := range []struct {
		t    *Trace
		port *Port
	}{{p.From, a}, {p.To, b}} {
		if side.port.Neutron.Status != "ACTIVE" {
			e, _ := Explain("port-not-active")
			return Check{Name: "ports", Status: StatusFail, Code: e.Code, Hint: e.Check,
				Detail: fmt.Sprintf("port %s of %s is %s", side.port.ID, Name(side.t), side.port.Neutron.Status)}
		}
	}
	return Check{Name: "ports", Status: StatusOK, Detail: "both ports are ACTIVE"}
}

// network checks L3: the same subnet needs nothing, different subnets
// need one Neutron router with an interface on both networks.
func (p *Path) network(a, b *Port) Check {
	na, nb := a.Neutron.Network, b.Neutron.Network
	if na == nil || nb == nil {
		return Check{Name: "network", Status: StatusSkip, Detail: "a network lookup failed during the trace"}
	}
	for _, s := range a.Neutron.Subnets {
		if _, cidr, err := net.ParseCIDR(s.CIDR); err == nil && cidr.Contains(net.ParseIP(p.DstIP)) {
			return Check{Name: "network", Status: StatusOK, Detail: fmt.Sprintf("same subnet %s on %s, no router needed", s.CIDR, na.Name)}
		}
	}
	for _, ra := range na.Routers {
		for _, rb := range nb.Routers {
			if ra.ID == rb.ID {
				return Check{Name: "network", Status: StatusOK, Detail: fmt.Sprintf("router %s connects %s and %s", or(ra.Name, ra.ID), na.Name, nb.Name)}
			}
		}
	}
	return Check{Name: "network", Status: StatusFail, Code: "path-no-router",
		Detail: fmt.Sprintf("no router has an interface on both %s and %s", na.Name, nb.Name),
		Hint:   fmt.Sprintf("Attach both subnets to one router: `openstack router add subnet <router> <subnet>`, for %s and %s.", na.Name, nb.Name)}
}

// securityGroup checks one direction. Security groups are stateful, so the
// reply needs no rule: egress on the source and ingress on the destination
// are enough.
func (p *Path) securityGroup(dir string, owner *Trace, port, peer *Port, peerIP string) Check {
	if !port.Neutron.PortSecurity {
		return Check{Name: dir, Status: StatusOK, Detail: fmt.Sprintf("port security is off on %s, so no security group filters it", Name(owner))}
	}
	var peerGroups, groups []string
	for _, g := range peer.Neutron.SecurityGroups {
		peerGroups = append(peerGroups, g.ID)
	}
	for _, g := range port.Neutron.SecurityGroups {
		groups = append(groups, g.Name)
		for _, r := range g.Rules {
			if r.Direction == dir && p.allows(r, peerIP, peerGroups) {
				return Check{Name: dir, Status: StatusOK, Detail: fmt.Sprintf("%s group %s allows it: %s", Name(owner), g.Name, r.String())}
			}
		}
	}

	target := "<group>"
	if len(groups) > 0 {
		target = groups[0]
	}
	flag, word := "--ingress", "from"
	if dir == "egress" {
		flag, word = "--egress", "to"
	}
	cmd := fmt.Sprintf("openstack security group rule create %s --protocol %s", flag, p.Protocol)
	if p.Protocol != "icmp" {
		cmd += fmt.Sprintf(" --dst-port %d", p.Port)
	}
	cmd += fmt.Sprintf(" --remote-ip %s/32 %s", peerIP, target)
	return Check{Name: dir, Status: StatusFail, Code: "sg-" + dir + "-blocked",
		Detail: fmt.Sprintf("no %s rule in %s of %s allows %s %s %s", dir, or(strings.Join(groups, ", "), "any group"), Name(owner), p.traffic(), word, peerIP),
		Hint:   "To allow it: `" + cmd + "`"}
}

func (p *Path) traffic() string {
	if p.Protocol == "icmp" {
		return "icmp echo"
	}
	return fmt.Sprintf("%s %d", p.Protocol, p.Port)
}

// protocolNames maps the numbers Neutron also accepts to their names.
var protocolNames = map[string]string{"1": "icmp", "6": "tcp", "17": "udp"}

// allows reports whether rule r admits this path's traffic from or to
// peerIP. peerGroups are the security groups of the peer's port, which a
// remote_group_id rule matches.
func (p *Path) allows(r Rule, peerIP string, peerGroups []string) bool {
	if r.EtherType != "" && r.EtherType != "IPv4" {
		return false
	}
	proto := r.Protocol
	if n, ok := protocolNames[proto]; ok {
		proto = n
	}
	if proto != "" && proto != "any" && proto != p.Protocol {
		return false
	}
	if proto != "" && proto != "any" {
		switch p.Protocol {
		case "icmp":
			if r.PortMin != nil && *r.PortMin != 8 { // 8 is echo request
				return false
			}
		default:
			lo, hi := r.PortMin, r.PortMax
			if hi == nil {
				hi = lo
			}
			if lo != nil && (p.Port < *lo || p.Port > *hi) {
				return false
			}
		}
	}
	switch {
	case r.RemoteIPPrefix != "":
		_, cidr, err := net.ParseCIDR(r.RemoteIPPrefix)
		return err == nil && cidr.Contains(net.ParseIP(peerIP))
	case r.RemoteGroupID != "":
		return slices.Contains(peerGroups, r.RemoteGroupID)
	}
	return true
}

// datapath asks the source compute's vRouter agent how it forwards the
// destination IP from the source VRF. It needs OpenSDN on both sides.
func (p *Path) datapath(ctx context.Context, hc *httpx.Client, a, b *Port) []Check {
	from, to := p.From, p.To
	if from.Compute == nil || from.Compute.AgentURL == "" || a.VRouter == nil || a.VRouter.Interface == nil {
		reason := "needs the vrouter stage of " + Name(from)
		if a.Neutron.VIFType != "" && a.Neutron.VIFType != "vrouter" {
			reason = fmt.Sprintf("ports use vif_type %s; plumb checks the datapath only on OpenSDN", a.Neutron.VIFType)
		}
		return []Check{{Name: "route", Status: StatusSkip, Detail: reason}}
	}

	vrf := a.VRouter.Interface.VRF
	routes, err := agent.New(hc, from.Compute.AgentURL).Routes(ctx, a.VRouter.VRFIndex, p.DstIP)
	if err != nil {
		iss := Classify(err)
		return []Check{{Name: "route", Status: StatusFail, Detail: iss.Message, Code: iss.Code, Hint: iss.Hint}}
	}
	var paths []agent.Path
	for _, r := range routes {
		paths = append(paths, r.Paths...)
	}
	if len(paths) == 0 {
		return []Check{{Name: "route", Status: StatusFail, Code: "path-no-route",
			Detail: fmt.Sprintf("VRF %s on %s has no route for %s", vrf, or(from.Compute.Name, from.Compute.IP), p.DstIP),
			Hint:   fmt.Sprintf("The routing instance of %s must import the route target of %s: check the network policy or router between them, then `plumb trace %s`.", Name(from), Name(to), Name(to))}}
	}
	route := Check{Name: "route", Status: StatusOK, Detail: fmt.Sprintf("VRF %s has %s/32 via %s", vrf, p.DstIP, describe(paths[0]))}

	if to.Compute == nil || to.Compute.IP == "" || b.VRouter == nil || b.VRouter.Interface == nil {
		return []Check{route, {Name: "next-hop", Status: StatusSkip, Detail: "the compute or interface of " + Name(to) + " is unknown"}}
	}
	itf := b.VRouter.Interface
	if from.Compute.IP == to.Compute.IP {
		for _, pa := range paths {
			if strings.EqualFold(pa.NHType, "interface") && pa.Interface == itf.Name {
				return []Check{route, {Name: "next-hop", Status: StatusOK, Detail: fmt.Sprintf("delivered to %s on the same compute", itf.Name)}}
			}
		}
		return []Check{route, {Name: "next-hop", Status: StatusFail, Code: "path-wrong-next-hop",
			Detail: fmt.Sprintf("%s runs on the same compute, but no path delivers to %s", Name(to), itf.Name),
			Hint:   fmt.Sprintf("Run `plumb trace %s` and check its vRouter interface.", Name(to))}}
	}
	for _, pa := range paths {
		if !strings.EqualFold(pa.NHType, "tunnel") || pa.TunnelDst != to.Compute.IP {
			continue
		}
		if pa.Label != itf.Label {
			e, _ := Explain("label-mismatch")
			return []Check{route, {Name: "next-hop", Status: StatusFail, Code: e.Code, Hint: e.Check,
				Detail: fmt.Sprintf("tunnel to %s uses label %s, but %s's interface has label %s", to.Compute.IP, pa.Label, Name(to), itf.Label)}}
		}
		return []Check{route, {Name: "next-hop", Status: StatusOK,
			Detail: fmt.Sprintf("%s tunnel to %s, label %s matches %s", or(pa.TunnelType, "overlay"), to.Compute.IP, pa.Label, itf.Name)}}
	}
	return []Check{route, {Name: "next-hop", Status: StatusFail, Code: "path-wrong-next-hop",
		Detail: fmt.Sprintf("no path tunnels to %s, where %s runs; first path is %s", to.Compute.IP, Name(to), describe(paths[0])),
		Hint:   fmt.Sprintf("The control nodes may hold a stale route. Run `plumb trace %s` and compare the Control lines.", Name(to))}}
}

func describe(pa agent.Path) string {
	switch strings.ToLower(pa.NHType) {
	case "interface":
		return "local interface " + pa.Interface
	case "tunnel":
		return fmt.Sprintf("%s tunnel to %s, label %s", or(pa.TunnelType, "overlay"), pa.TunnelDst, pa.Label)
	}
	return "next hop " + pa.NHType
}
