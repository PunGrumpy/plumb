package trace

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"plumb/internal/httpx"
	"plumb/internal/opensdn/agent"
	"plumb/internal/opensdn/config"
	"plumb/internal/opensdn/control"
)

var errNoVMI = errors.New("no virtual-machine-interface with this UUID; Neutron may not use the OpenSDN plugin, or the two are out of sync")

// config maps each Neutron port to its OpenSDN objects, finds the compute
// node's virtual-router and lists the control nodes.
func (r *run) config(ctx context.Context) error {
	if r.opts.ConfigURL == "" {
		if vif := r.nonVRouterVIF(); vif != "" {
			return skip("ports use vif_type %s, so this cloud does not run OpenSDN", vif)
		}
		return skip("no Config API URL; run plumb link if the cloud runs OpenSDN")
	}
	if len(r.t.Ports) == 0 {
		return skip("no ports to look up")
	}
	token := ""
	if r.opts.ConfigToken && r.sess != nil {
		token = r.sess.Token
	}
	c := config.New(r.opts.HTTP, r.opts.ConfigURL, token)

	resolved := 0
	for _, p := range r.t.Ports {
		ch, err := r.configChain(ctx, c, p.ID)
		if err != nil {
			code := "lookup-failed"
			if errors.Is(err, errNoVMI) {
				code = "vmi-missing"
			}
			r.warn(code, "port %s: %v", p.ID, err)
			continue
		}
		p.Config = ch
		resolved++
	}
	if resolved == 0 {
		return fail("vmi-missing", "no port resolved to a virtual-machine-interface")
	}

	r.findCompute(ctx, c)

	if len(r.opts.ControlURLs) > 0 {
		for _, u := range r.opts.ControlURLs {
			r.t.ControlNodes = append(r.t.ControlNodes, ControlNode{URL: u})
		}
		return nil
	}
	nodes, err := c.ControlNodes(ctx)
	if err != nil {
		r.warn("control-discovery", "bgp-routers: %v", err)
	}
	for _, n := range nodes {
		r.t.ControlNodes = append(r.t.ControlNodes, ControlNode{
			Name:    n.FQName[len(n.FQName)-1],
			Address: n.Params.Address,
			URL:     fmt.Sprintf("http://%s:%d", n.Params.Address, r.opts.ControlPort),
		})
	}
	return nil
}

// nonVRouterVIF returns the vif_type shared by every port when Neutron wired
// all of them without OpenSDN, such as "ovs" on DevStack with OVN.
func (r *run) nonVRouterVIF() string {
	vif := ""
	for _, p := range r.t.Ports {
		if p.Neutron == nil || p.Neutron.VIFType == "vrouter" || p.Neutron.VIFType == "" {
			return ""
		}
		if vif != "" && vif != p.Neutron.VIFType {
			return ""
		}
		vif = p.Neutron.VIFType
	}
	return vif
}

// configChain follows VMI → virtual-network → routing-instance →
// route-targets. The VMI UUID is the Neutron port ID.
func (r *run) configChain(ctx context.Context, c *config.Client, portID string) (*ConfigChain, error) {
	vmi, err := c.VMI(ctx, portID)
	if err != nil {
		if httpx.IsNotFound(err) {
			return nil, errNoVMI
		}
		return nil, err
	}
	ch := &ConfigChain{VMI: Object{UUID: vmi.UUID, FQName: vmi.Name()}}

	for _, ref := range vmi.SecurityGroupRefs {
		ch.SecurityGroups = append(ch.SecurityGroups, ref.FQName())
	}
	for _, ref := range vmi.InstanceIPBackRefs {
		iip, err := c.InstanceIP(ctx, ref.UUID)
		if err != nil {
			r.warn("lookup-failed", "instance-ip %s: %v", ref.UUID, err)
			continue
		}
		ch.InstanceIPs = append(ch.InstanceIPs, iip.Address)
	}
	for _, ref := range vmi.FloatingIPBackRefs {
		fip, err := c.FloatingIP(ctx, ref.UUID)
		if err != nil {
			r.warn("lookup-failed", "floating-ip %s: %v", ref.UUID, err)
			continue
		}
		ch.FloatingIPs = append(ch.FloatingIPs, fip.Address)
	}

	var vn *config.VirtualNetwork
	if len(vmi.VirtualNetworkRefs) > 0 {
		vn, err = c.VirtualNetwork(ctx, vmi.VirtualNetworkRefs[0].UUID)
		if err != nil {
			r.warn("lookup-failed", "virtual-network %s: %v", vmi.VirtualNetworkRefs[0].UUID, err)
		}
	}
	if vn != nil {
		v := &VN{
			Object:           Object{UUID: vn.UUID, FQName: vn.Name()},
			NetworkID:        vn.NetworkID,
			VxlanID:          vn.Properties.VxlanID,
			ForwardingMode:   vn.Properties.ForwardingMode,
			UserRouteTargets: vn.RouteTargetList.RouteTarget,
		}
		for _, ref := range vn.NetworkPolicyRefs {
			v.Policies = append(v.Policies, ref.FQName())
		}
		ch.VirtualNetwork = v
	}

	ri := primaryRI(vmi, vn)
	if ri == nil {
		r.warn("no-routing-instance", "port %s: no routing-instance; the schema transformer has not processed this network", portID)
		return ch, nil
	}
	ch.RoutingInstance = &Object{UUID: ri.UUID, FQName: ri.FQName()}
	obj, err := c.RoutingInstance(ctx, ri.UUID)
	if err != nil {
		r.warn("lookup-failed", "routing-instance %s: %v", ri.UUID, err)
		return ch, nil
	}
	for _, ref := range obj.RouteTargetRefs {
		ch.RouteTargets = append(ch.RouteTargets, RouteTarget{Name: ref.FQName(), Mode: ref.ImportExport()})
	}
	if len(ch.RouteTargets) == 0 {
		r.warn("no-route-target", "routing-instance %s has no route-target", ri.FQName())
	}
	return ch, nil
}

// primaryRI prefers the VMI's own ref and falls back to the network's child
// whose name matches the network name, which is how the schema transformer
// names the primary instance.
func primaryRI(vmi *config.VMI, vn *config.VirtualNetwork) *config.Ref {
	if len(vmi.RoutingInstanceRefs) > 0 {
		return &vmi.RoutingInstanceRefs[0]
	}
	if vn == nil || len(vn.FQName) == 0 {
		return nil
	}
	want := vn.Name() + ":" + vn.FQName[len(vn.FQName)-1]
	for i := range vn.RoutingInstances {
		if vn.RoutingInstances[i].FQName() == want {
			return &vn.RoutingInstances[i]
		}
	}
	return nil
}

// findCompute follows virtual-machine → virtual-router to learn which
// compute node runs the VM and the IP its agent listens on.
func (r *run) findCompute(ctx context.Context, c *config.Client) {
	vm, err := c.VirtualMachine(ctx, r.t.VMID)
	if err != nil {
		r.warn("compute-unknown", "virtual-machine %s: %v", r.t.VMID, err)
		return
	}
	if len(vm.VirtualRouterBackRefs) == 0 {
		r.warn("compute-unknown", "virtual-machine %s is not attached to any virtual-router", r.t.VMID)
		return
	}
	vr, err := c.VirtualRouter(ctx, vm.VirtualRouterBackRefs[0].UUID)
	if err != nil {
		r.warn("compute-unknown", "virtual-router: %v", err)
		return
	}
	r.t.Compute = &Compute{Name: vr.FQName[len(vr.FQName)-1], IP: vr.IPAddress}
	if r.t.Server != nil && r.t.Server.Host != "" && shortHost(r.t.Server.Host) != shortHost(r.t.Compute.Name) {
		r.warn("host-mismatch", "Nova runs the VM on %s but OpenSDN places it on %s", r.t.Server.Host, r.t.Compute.Name)
	}
}

// control asks every control node for each VM prefix in the port's
// routing instance and checks the XMPP session with the compute node.
func (r *run) control(ctx context.Context) error {
	if len(r.t.ControlNodes) == 0 {
		if r.opts.ConfigURL == "" {
			return skip("needs opensdn-config")
		}
		return skip("no control nodes known; pass --control-url")
	}
	var targets []*Port
	for _, p := range r.t.Ports {
		if p.Config != nil && p.Config.RoutingInstance != nil && len(p.IPs) > 0 {
			targets = append(targets, p)
		}
	}
	if len(targets) == 0 {
		return skip("no routing-instance resolved in config")
	}

	reachable := 0
	for i := range r.t.ControlNodes {
		cn := &r.t.ControlNodes[i]
		label := cn.Name
		if label == "" {
			label = cn.URL
		}
		c := control.New(r.opts.HTTP, cn.URL)

		nbrs, err := c.Neighbors(ctx)
		if err != nil {
			r.warn("control-unreachable", "%s: %v", label, err)
			continue
		}
		reachable++
		if r.t.Compute != nil {
			cn.XMPPState = xmppState(nbrs, r.t.Compute)
			if cn.XMPPState == "" {
				r.warn("xmpp-missing", "%s has no XMPP session with %s", label, r.t.Compute.Name)
			} else if cn.XMPPState != "Established" {
				r.warn("xmpp-missing", "%s: XMPP session with %s is %s", label, r.t.Compute.Name, cn.XMPPState)
			}
		}

		for _, p := range targets {
			for _, ip := range p.IPs {
				table, prefix := p.Config.RoutingInstance.FQName+".inet.0", ip+"/32"
				if strings.Contains(ip, ":") {
					table, prefix = p.Config.RoutingInstance.FQName+".inet6.0", ip+"/128"
				}
				cr := ControlRoute{ControlNode: label, Table: table, Prefix: prefix}
				routes, err := c.Routes(ctx, table, prefix)
				if err != nil {
					r.warn("lookup-failed", "%s: %v", label, err)
				}
				for _, rt := range routes {
					for _, pa := range rt.Paths {
						cr.Paths = append(cr.Paths, ControlPath{
							Protocol: pa.Protocol,
							Source:   pa.Source,
							NextHop:  pa.NextHop,
							Label:    pa.Label,
							Encap:    pa.Encap,
							OriginVN: pa.OriginVN,
						})
					}
				}
				cr.Found = len(cr.Paths) > 0
				if !cr.Found && err == nil {
					r.warn("route-missing", "%s: %s is not in %s; the agent has not advertised it over XMPP", label, prefix, table)
				}
				p.Control = append(p.Control, cr)
			}
		}
	}
	if reachable == 0 {
		return fail("control-unreachable", "no control node introspect is reachable")
	}
	return nil
}

func xmppState(nbrs []control.Neighbor, cmp *Compute) string {
	for _, n := range nbrs {
		if n.Encoding != "XMPP" {
			continue
		}
		if (cmp.IP != "" && n.Address == cmp.IP) || (cmp.Name != "" && n.Peer == cmp.Name) {
			return n.State
		}
	}
	return ""
}

// vrouter reads the agent on the VM's compute node: its XMPP sessions, the
// VM's tap interface, the VRF it sits in, the route for each VM IP and the
// flows touching those IPs.
func (r *run) vrouter(ctx context.Context) error {
	base := r.opts.AgentURL
	if base == "" {
		if r.t.Compute == nil || r.t.Compute.IP == "" {
			if r.opts.ConfigURL == "" {
				return skip("needs opensdn-config")
			}
			return skip("compute IP unknown; needs a virtual-router in config, or pass --agent-url")
		}
		base = fmt.Sprintf("http://%s:%d", r.t.Compute.IP, r.opts.AgentPort)
	}
	if r.t.Compute == nil {
		r.t.Compute = &Compute{}
	}
	r.t.Compute.AgentURL = base
	c := agent.New(r.opts.HTTP, base)

	peers, err := c.XMPPPeers(ctx)
	if err != nil {
		return err
	}
	up := 0
	for _, p := range peers {
		r.t.Compute.XMPP = append(r.t.Compute.XMPP, XMPPSession{Controller: p.Controller, State: p.State, Config: p.Config})
		if p.State == "Established" {
			up++
		}
	}
	if up == 0 {
		r.warn("agent-xmpp-down", "agent has no Established XMPP session, so no control node learns its routes")
	}

	flows, err := c.Flows(ctx)
	if err != nil {
		r.warn("lookup-failed", "flows: %v", err)
	}

	for _, p := range r.t.Ports {
		v := &VRouterView{}
		p.VRouter = v
		itf, err := c.Interface(ctx, p.ID)
		if err != nil {
			r.warn("interface-missing", "port %s: %v", p.ID, err)
			continue
		}
		v.Interface = &AgentInterface{Name: itf.Name, Active: itf.Active == "Active", VRF: itf.VRF, Label: itf.Label}
		if !v.Interface.Active {
			r.warn("interface-inactive", "interface %s is %s", itf.Name, itf.Active)
		}
		r.checkLabel(p, itf.Label)

		vrf, err := c.VRF(ctx, itf.VRF)
		if err != nil {
			r.warn("lookup-failed", "port %s: %v", p.ID, err)
			continue
		}
		v.VRFIndex = vrf.UCIndex

		for _, ip := range p.IPs {
			if strings.Contains(ip, ":") {
				continue
			}
			routes, err := c.Routes(ctx, vrf.UCIndex, ip)
			if err != nil {
				r.warn("lookup-failed", "route %s: %v", ip, err)
				continue
			}
			if len(routes) == 0 {
				r.warn("agent-route-missing", "VRF %s has no route for %s", itf.VRF, ip)
			}
			for _, rt := range routes {
				ar := AgentRoute{Prefix: rt.Prefix}
				for _, pa := range rt.Paths {
					ar.Paths = append(ar.Paths, AgentPath{
						Peer:       pa.Peer,
						NHType:     pa.NHType,
						Interface:  pa.Interface,
						TunnelDst:  pa.TunnelDst,
						TunnelType: pa.TunnelType,
						Label:      pa.Label,
					})
				}
				v.Routes = append(v.Routes, ar)
			}
		}

		for _, f := range flows {
			if !slices.Contains(p.IPs, f.SrcIP) && !slices.Contains(p.IPs, f.DstIP) {
				continue
			}
			v.FlowsSampled++
			if len(v.Flows) < 10 {
				v.Flows = append(v.Flows, Flow{
					Src:        f.SrcIP + ":" + f.SrcPort,
					Dst:        f.DstIP + ":" + f.DstPort,
					Protocol:   f.Protocol,
					DropReason: f.DropReason,
				})
			}
		}
	}
	return nil
}

// checkLabel compares the label the agent assigned to the interface with
// the label control nodes advertise for routes from this compute. They
// must match, or remote computes send packets the agent cannot deliver.
func (r *run) checkLabel(p *Port, label string) {
	if r.t.Compute == nil || r.t.Compute.IP == "" {
		return
	}
	for _, cr := range p.Control {
		for _, pa := range cr.Paths {
			if pa.NextHop == r.t.Compute.IP && pa.Label != label {
				r.warn("label-mismatch", "%s advertises %s with label %s, agent assigned %s", cr.ControlNode, cr.Prefix, pa.Label, label)
			}
		}
	}
}
