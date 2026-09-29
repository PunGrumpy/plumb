package trace

import (
	"context"
	"fmt"
	"net"
	"strings"

	"plumb/internal/glance"
	"plumb/internal/httpx"
	"plumb/internal/keystone"
	"plumb/internal/neutron"
	"plumb/internal/nova"
)

// keystone gets a token and resolves the endpoints the next stages call.
func (r *run) keystone(ctx context.Context) error {
	if err := r.opts.Creds.Validate(); err != nil {
		return fail("no-credentials", "%v", err)
	}
	s, err := keystone.Authenticate(ctx, r.opts.HTTP, r.opts.Creds)
	if err != nil {
		return err
	}
	r.sess = s

	id := &Identity{
		User:      s.User,
		Project:   s.Project,
		Roles:     s.Roles,
		Admin:     s.HasRole("admin"),
		Endpoints: map[string]string{},
	}
	for _, typ := range []string{"compute", "network", "image"} {
		u, err := s.Endpoint(typ, r.opts.Creds.Interface, r.opts.Creds.Region)
		if err != nil {
			r.warn("endpoint-missing", "%v", err)
			continue
		}
		id.Endpoints[typ] = u
	}
	if !id.Admin {
		r.warn("no-admin", "token has no admin role, so Nova hides the host field")
	}
	r.t.Identity = id
	return nil
}

// nova reads where the VM runs and which ports Nova attached to it.
func (r *run) nova(ctx context.Context) error {
	ep := r.endpoint("compute")
	if ep == "" {
		return skip("no compute endpoint")
	}
	c := nova.New(r.opts.HTTP, ep, r.sess.Token)
	switch {
	case net.ParseIP(r.t.VMID) != nil:
		if err := r.resolveIP(ctx); err != nil {
			return err
		}
	case !IsUUID(r.t.VMID):
		if err := r.resolveName(ctx, c); err != nil {
			return err
		}
	}
	srv, err := c.Server(ctx, r.t.VMID)
	if httpx.IsNotFound(err) {
		return fail("vm-not-found", "server %s not found, or not visible to project %s", r.t.VMID, r.sess.Project)
	}
	if err != nil {
		return err
	}
	r.t.Server = &Server{
		ID:           srv.ID,
		Name:         srv.Name,
		Status:       srv.Status,
		Host:         srv.Host,
		InstanceName: srv.InstanceName,
		Flavor: Flavor{
			Name:   srv.Flavor.Name,
			VCPUs:  srv.Flavor.VCPUs,
			RAMMB:  srv.Flavor.RAM,
			DiskGB: srv.Flavor.Disk,
		},
		ImageID: srv.Image.ID,
	}
	if srv.Status != "ACTIVE" {
		r.warn("server-not-active", "server status is %s, not ACTIVE", srv.Status)
	}

	if img := r.endpoint("image"); img != "" && srv.Image.ID != "" {
		name, err := glance.New(r.opts.HTTP, img, r.sess.Token).ImageName(ctx, srv.Image.ID)
		if err != nil {
			r.warn("lookup-failed", "image %s: %v", srv.Image.ID, err)
		}
		r.t.Server.ImageName = name
	}

	ifaces, err := c.Interfaces(ctx, r.t.VMID)
	if err != nil {
		r.warn("lookup-failed", "os-interface: %v", err)
		return nil
	}
	for _, i := range ifaces {
		p := r.port(i.PortID)
		p.MAC = i.MACAddr
		for _, ip := range i.FixedIPs {
			p.IPs = appendUnique(p.IPs, ip.IPAddress)
		}
	}
	return nil
}

// resolveIP swaps a fixed or floating IP for the UUID of the VM that uses
// it. Neutron knows both: a fixed IP sits on a port, and a floating IP
// points at one. The port's device_id is the VM when the port belongs to
// Nova, whose device_owner starts with "compute:".
func (r *run) resolveIP(ctx context.Context) error {
	ip := r.t.VMID
	ep := r.endpoint("network")
	if ep == "" {
		return fail("endpoint-missing", "looking up IP %s needs the network endpoint", ip)
	}
	c := neutron.New(r.opts.HTTP, ep, r.sess.Token)
	ports, err := c.PortsByFixedIP(ctx, ip)
	if err != nil {
		return err
	}
	vms, others := vmOwners(ports)

	// Neutron also holds each floating IP as the fixed IP of a
	// network:floatingip port on the external network, so a miss among VM
	// ports still needs a floating IP lookup.
	if len(vms) == 0 {
		fips, err := c.FloatingIPsByAddress(ctx, ip)
		if err != nil {
			return err
		}
		switch {
		case len(fips) == 0 && len(ports) == 0:
			return fail("ip-not-found", "no port has fixed or floating IP %s", ip)
		case len(fips) > 0 && fips[0].PortID == "":
			return fail("ip-not-vm", "floating IP %s is not associated with any port", ip)
		case len(fips) > 0:
			p, err := c.Port(ctx, fips[0].PortID)
			if err != nil {
				return err
			}
			vms, others = vmOwners([]neutron.Port{*p})
		}
	}

	switch len(vms) {
	case 0:
		return fail("ip-not-vm", "IP %s belongs to %s, not a VM", ip, strings.Join(others, "; "))
	case 1:
		r.t.VMID = vms[0]
		return nil
	}
	return fail("vm-ambiguous", "%d VMs use IP %s: %s", len(vms), ip, strings.Join(vms, ", "))
}

// vmOwners splits ports into the VMs that own them and a description of
// every other owner, such as a router interface.
func vmOwners(ports []neutron.Port) (vms, others []string) {
	for _, p := range ports {
		if strings.HasPrefix(p.DeviceOwner, "compute:") && p.DeviceID != "" {
			vms = appendUnique(vms, p.DeviceID)
			continue
		}
		owner := p.DeviceOwner
		if owner == "" {
			owner = "nothing"
		}
		others = append(others, fmt.Sprintf("port %s owned by %s", p.ID, owner))
	}
	return vms, others
}

// resolveName swaps the server name the user typed for its UUID. It
// searches the token's project first; an admin token then searches every
// project, because an admin often traces VMs that other projects own.
func (r *run) resolveName(ctx context.Context, c *nova.Client) error {
	servers, err := c.FindByName(ctx, r.t.VMID, false)
	if err != nil {
		return err
	}
	if len(servers) == 0 && r.sess.HasRole("admin") {
		if servers, err = c.FindByName(ctx, r.t.VMID, true); err != nil {
			return err
		}
	}
	switch len(servers) {
	case 0:
		if r.sess.HasRole("admin") {
			return fail("vm-not-found", "no server named %q in any project", r.t.VMID)
		}
		return fail("vm-not-found", "no server named %q in project %s", r.t.VMID, r.sess.Project)
	case 1:
		r.t.VMID = servers[0].ID
		return nil
	}
	var ids []string
	for _, s := range servers {
		ids = append(ids, s.ID)
	}
	return fail("vm-ambiguous", "%d servers are named %q: %s", len(servers), r.t.VMID, strings.Join(ids, ", "))
}

// neutron reads each port and the network objects around it. It looks
// ports up by device_id, so it still works when the nova stage failed.
func (r *run) neutron(ctx context.Context) error {
	ep := r.endpoint("network")
	if ep == "" {
		return skip("no network endpoint")
	}
	if !IsUUID(r.t.VMID) {
		return skip("%q was not resolved to a VM UUID", r.t.VMID)
	}
	c := neutron.New(r.opts.HTTP, ep, r.sess.Token)
	ports, err := c.PortsByDevice(ctx, r.t.VMID)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		r.warn("no-ports", "no port has device_id=%s", r.t.VMID)
	}

	for _, np := range ports {
		p := r.port(np.ID)
		p.MAC = np.MACAddress
		v := &NeutronPort{
			Status:       np.Status,
			VIFType:      np.VIFType,
			VNICType:     np.VNICType,
			BindingHost:  np.HostID,
			PortSecurity: np.PortSecurity == nil || *np.PortSecurity,
		}
		p.Neutron = v

		switch {
		case np.VIFType == "binding_failed":
			r.warn("binding-failed", "port %s: binding failed, no mechanism driver could wire it on %s", np.ID, np.HostID)
		case np.Status != "ACTIVE":
			r.warn("port-not-active", "port %s: status is %s, not ACTIVE", np.ID, np.Status)
		}
		if r.t.Server != nil && r.t.Server.Host != "" && shortHost(np.HostID) != shortHost(r.t.Server.Host) {
			r.warn("host-mismatch", "port %s is bound to %s but Nova runs the VM on %s", np.ID, np.HostID, r.t.Server.Host)
		}

		if n, err := c.Network(ctx, np.NetworkID); err != nil {
			r.warn("lookup-failed", "network %s: %v", np.NetworkID, err)
		} else {
			v.Network = &Network{ID: n.ID, Name: n.Name, Type: n.NetworkType, SegmentationID: n.SegmentationID, MTU: n.MTU}
			v.Network.Routers = r.routers(ctx, c, n.ID)
		}

		for _, ip := range np.FixedIPs {
			p.IPs = appendUnique(p.IPs, ip.IPAddress)
			s, err := c.Subnet(ctx, ip.SubnetID)
			if err != nil {
				r.warn("lookup-failed", "subnet %s: %v", ip.SubnetID, err)
				continue
			}
			v.Subnets = append(v.Subnets, Subnet{ID: s.ID, CIDR: s.CIDR, Gateway: s.GatewayIP})
		}

		for _, id := range np.SecurityGroups {
			sg, err := c.SecurityGroup(ctx, id)
			if err != nil {
				r.warn("lookup-failed", "security group %s: %v", id, err)
				continue
			}
			g := SecurityGroup{ID: sg.ID, Name: sg.Name}
			for _, rule := range sg.Rules {
				g.Rules = append(g.Rules, toRule(rule))
			}
			v.SecurityGroups = append(v.SecurityGroups, g)
		}

		fips, err := c.FloatingIPsByPort(ctx, np.ID)
		if err != nil {
			r.warn("lookup-failed", "floating IPs of %s: %v", np.ID, err)
		}
		for _, f := range fips {
			v.FloatingIPs = append(v.FloatingIPs, f.FloatingIPAddress)
		}
	}
	return nil
}

// routers lists the routers attached to a network, by name when Neutron
// returns it.
func (r *run) routers(ctx context.Context, c *neutron.Client, networkID string) []Router {
	ids, err := c.RouterIDs(ctx, networkID)
	if err != nil {
		r.warn("lookup-failed", "routers of network %s: %v", networkID, err)
		return nil
	}
	var out []Router
	for _, id := range ids {
		rt := Router{ID: id}
		if got, err := c.Router(ctx, id); err == nil {
			rt.Name = got.Name
		}
		out = append(out, rt)
	}
	return out
}

func toRule(r neutron.Rule) Rule {
	out := Rule{Direction: r.Direction, EtherType: r.EtherType}
	if r.Protocol != nil {
		out.Protocol = *r.Protocol
	}
	out.PortMin, out.PortMax = r.PortRangeMin, r.PortRangeMax
	if r.RemoteIPPrefix != nil {
		out.RemoteIPPrefix = *r.RemoteIPPrefix
	}
	if r.RemoteGroupID != nil {
		out.RemoteGroupID = *r.RemoteGroupID
	}
	return out
}
