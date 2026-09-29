// Package render prints a Trace as a terminal tree or as JSON.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/PunGrumpy/plumb/apps/cli/internal/trace"
	"github.com/PunGrumpy/plumb/apps/cli/internal/ui"
)

type Options struct {
	Timings bool
	Color   bool
}

// JSON writes the trace as indented JSON.
func JSON(w io.Writer, t *trace.Trace) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(t)
}

type node struct {
	label string
	text  string
	kids  []*node
}

func (n *node) add(label, format string, a ...any) *node {
	k := &node{label: label, text: fmt.Sprintf(format, a...)}
	n.kids = append(n.kids, k)
	return k
}

// print writes the children of n. Layer labels at depth 1 are bold, the
// tree lines are dim and status marks are colored.
func (n *node) print(w io.Writer, p ui.Palette, prefix string, depth int) {
	marks := strings.NewReplacer("✓", p.Green("✓"), "✗", p.Red("✗"))
	for i, k := range n.kids {
		branch, next := "├─ ", "│  "
		if i == len(n.kids)-1 {
			branch, next = "└─ ", "   "
		}
		label := fmt.Sprintf("%-9s", k.label)
		if depth == 1 {
			label = p.Bold(label)
		} else {
			label = p.Dim(label)
		}
		text := marks.Replace(strings.TrimRight(k.text, " "))
		fmt.Fprintf(w, "%s%s %s\n", p.Dim(prefix+branch), label, text)
		k.print(w, p, prefix+next, depth+1)
	}
}

// Tree writes the VM → layer → object tree, then one line per stage.
func Tree(w io.Writer, t *trace.Trace, o Options) {
	p := ui.Palette{On: o.Color}
	root := &node{}
	if s := t.Server; s != nil {
		root.text = fmt.Sprintf("VM %s (%s)  %s", s.Name, s.ID, s.Status)
	} else {
		root.text = "VM " + t.VMID
	}

	if id := t.Identity; id != nil {
		role := ""
		if id.Admin {
			role = "  admin"
		}
		n := root.add("Keystone", "user %s in project %s%s", id.User, id.Project, role)
		for _, typ := range []string{"compute", "network", "image"} {
			if u := id.Endpoints[typ]; u != "" {
				n.add("endpoint", "%-8s %s", typ, u)
			}
		}
	}

	if s := t.Server; s != nil {
		n := root.add("Nova", "host %s  %s", or(s.Host, "(hidden)"), s.InstanceName)
		f := s.Flavor
		n.add("flavor", "%s  %d vCPU  %d MB RAM  %d GB disk", f.Name, f.VCPUs, f.RAMMB, f.DiskGB)
		if s.ImageID != "" {
			n.add("image", "%s (%s)", or(s.ImageName, "?"), s.ImageID)
		} else {
			n.add("image", "none, boots from volume")
		}
	}

	if c := t.Compute; c != nil {
		n := root.add("Compute", "%s  %s  (OpenSDN virtual-router)", or(c.Name, "?"), or(c.IP, "?"))
		if c.AgentURL != "" {
			n.add("agent", "%s", c.AgentURL)
		}
		for _, x := range c.XMPP {
			role := ""
			if x.Config {
				role = "  config"
			}
			n.add("XMPP", "→ %s  %s%s", x.Controller, x.State, role)
		}
	}

	for _, cn := range t.ControlNodes {
		root.add("Control", "%s  %s  XMPP with compute: %s", or(cn.Name, cn.URL), cn.Address, or(cn.XMPPState, "none"))
	}

	for _, p := range t.Ports {
		portNode(root, p)
	}

	fmt.Fprintln(w, p.Bold(root.text))
	root.print(w, p, "", 1)
	fmt.Fprintln(w)
	steps(w, t.Steps, o, p)
	fmt.Fprintln(w)
	summary(w, t, o, p)
}

func portNode(root *node, p *trace.Port) {
	pn := root.add("Port", "%s  %s  %s", p.ID, p.MAC, strings.Join(p.IPs, ", "))

	if np := p.Neutron; np != nil {
		n := pn.add("Neutron", "%s  vif_type=%s  host=%s", np.Status, np.VIFType, np.BindingHost)
		if net := np.Network; net != nil {
			seg := ""
			if net.SegmentationID != nil {
				seg = " " + strconv.Itoa(*net.SegmentationID)
			}
			nn := n.add("network", "%s  %s%s  mtu %d", net.Name, or(net.Type, "?"), seg, net.MTU)
			for _, rt := range net.Routers {
				nn.add("router", "%s (%s)", or(rt.Name, "?"), rt.ID)
			}
		}
		for _, s := range np.Subnets {
			n.add("subnet", "%s  gw %s", s.CIDR, or(s.Gateway, "none"))
		}
		for _, sg := range np.SecurityGroups {
			g := n.add("secgroup", "%s  %d rules", sg.Name, len(sg.Rules))
			for _, r := range sg.Rules {
				g.add("rule", "%s", r.String())
			}
		}
		for _, f := range np.FloatingIPs {
			n.add("floating", "%s", f)
		}
	}

	if ch := p.Config; ch != nil {
		same := "✗ differs from port ID"
		if ch.VMI.UUID == p.ID {
			same = "✓ same UUID as the port"
		}
		n := pn.add("Config", "VMI %s  %s", ch.VMI.FQName, same)
		if vn := ch.VirtualNetwork; vn != nil {
			v := n.add("VN", "%s  id %d", vn.FQName, vn.NetworkID)
			if vn.VxlanID != 0 {
				v.text += fmt.Sprintf("  vxlan %d", vn.VxlanID)
			}
			if vn.ForwardingMode != "" {
				v.text += "  mode " + vn.ForwardingMode
			}
			for _, rt := range vn.UserRouteTargets {
				v.add("user RT", "%s", rt)
			}
			for _, pol := range vn.Policies {
				v.add("policy", "%s", pol)
			}
		}
		if ri := ch.RoutingInstance; ri != nil {
			n.add("RI", "%s", ri.FQName)
		}
		for _, rt := range ch.RouteTargets {
			n.add("RT", "%s  %s", rt.Name, rt.Mode)
		}
		if len(ch.InstanceIPs) > 0 {
			n.add("IIP", "%s", strings.Join(ch.InstanceIPs, ", "))
		}
		if len(ch.FloatingIPs) > 0 {
			n.add("FIP", "%s", strings.Join(ch.FloatingIPs, ", "))
		}
	}

	for _, cr := range p.Control {
		mark := "✗ missing"
		if cr.Found {
			mark = "✓"
		}
		n := pn.add("Control", "%s  %s  %s %s", cr.ControlNode, shortTable(cr.Table), cr.Prefix, mark)
		for _, pa := range cr.Paths {
			n.add("path", "%s from %s  nh %s  label %s  encap %s",
				pa.Protocol, or(pa.Source, "?"), pa.NextHop, pa.Label, or(strings.Join(pa.Encap, ","), "?"))
		}
	}

	if v := p.VRouter; v != nil {
		if itf := v.Interface; itf != nil {
			state := "✗ inactive"
			if itf.Active {
				state = "✓ active"
			}
			n := pn.add("vRouter", "%s %s  vrf %s (index %d)  label %s", itf.Name, state, shortTable(itf.VRF), v.VRFIndex, itf.Label)
			for _, rt := range v.Routes {
				for _, pa := range rt.Paths {
					n.add("route", "%s  %s", rt.Prefix, nextHop(pa))
				}
			}
			if v.FlowsSampled > 0 {
				f := n.add("flows", "%d in the first page", v.FlowsSampled)
				for _, fl := range v.Flows {
					drop := ""
					if fl.DropReason != "" {
						drop = "  drop " + fl.DropReason
					}
					f.add("flow", "%s → %s  %s%s", fl.Src, fl.Dst, protoName(fl.Protocol), drop)
				}
			}
		} else {
			pn.add("vRouter", "✗ interface not found on agent")
		}
	}
}

func nextHop(pa trace.AgentPath) string {
	switch strings.ToLower(pa.NHType) {
	case "interface":
		return fmt.Sprintf("local interface %s  label %s", pa.Interface, pa.Label)
	case "tunnel":
		return fmt.Sprintf("tunnel %s to %s  label %s", pa.TunnelType, pa.TunnelDst, pa.Label)
	}
	return fmt.Sprintf("nh %s  peer %s  label %s", pa.NHType, pa.Peer, pa.Label)
}

// shortTable drops the domain and project from a routing instance name:
// default-domain:admin:vn1:vn1.inet.0 → vn1:vn1.inet.0
func shortTable(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) <= 2 {
		return name
	}
	return strings.Join(parts[len(parts)-2:], ":")
}

func steps(w io.Writer, ss []trace.Step, o Options, p ui.Palette) {
	fmt.Fprintln(w, p.Bold("Steps"))
	for _, s := range ss {
		line := fmt.Sprintf("%-15s", s.Name)
		if o.Timings && s.Status != trace.StatusSkip {
			line += fmt.Sprintf(" %5d ms", s.DurationMS)
		}
		switch s.Status {
		case trace.StatusSkip:
			line = p.Dim(line + "  skipped: " + s.Error)
		case trace.StatusFail:
			line += "  " + s.Error
		}
		fmt.Fprintf(w, "  %s %s\n", p.Mark(string(s.Status)), strings.TrimRight(line, " "))
		for _, iss := range s.Warnings {
			fmt.Fprintf(w, "      %s %s\n", p.Yellow("!"), iss.Message)
		}
	}
}

// summary ends the output with one verdict line. When something is wrong
// it names the first problem, the next thing to check and the code that
// `plumb explain` knows.
func summary(w io.Writer, t *trace.Trace, o Options, p ui.Palette) {
	name := t.Query
	if t.Server != nil && t.Server.Name != "" {
		name = t.Server.Name
	}
	var done, total int
	var ms int64
	var firstSkip, firstFail, firstWarn *trace.Step
	issues := 0
	for i := range t.Steps {
		s := &t.Steps[i]
		total++
		ms += s.DurationMS
		switch s.Status {
		case trace.StatusOK, trace.StatusWarn:
			done++
		case trace.StatusSkip:
			if firstSkip == nil {
				firstSkip = s
			}
		case trace.StatusFail:
			issues++
			if firstFail == nil {
				firstFail = s
			}
		}
		if len(s.Warnings) > 0 && firstWarn == nil {
			firstWarn = s
		}
		issues += len(s.Warnings)
	}
	took := ""
	if o.Timings {
		took = fmt.Sprintf(" in %d ms", ms)
	}

	switch {
	case firstFail != nil:
		fmt.Fprintf(w, "%s %s\n", p.Red("✗"), p.Bold(fmt.Sprintf("Trace of %s stopped at %s", name, firstFail.Name)))
		hint(w, p, trace.Issue{Code: firstFail.Code, Message: firstFail.Error, Hint: firstFail.Hint})
	case firstWarn != nil:
		fmt.Fprintf(w, "%s %s\n", p.Yellow("!"), p.Bold(fmt.Sprintf("Traced %s through %d of %d stages%s, %s", name, done, total, took, plural(issues, "issue"))))
		hint(w, p, firstWarn.Warnings[0])
	default:
		fmt.Fprintf(w, "%s %s\n", p.Green("✓"), p.Bold(fmt.Sprintf("Traced %s through %d of %d stages%s", name, done, total, took)))
		if firstSkip != nil {
			fmt.Fprintf(w, "  %s %s: %s\n", p.Dim("Skipped"), firstSkip.Name, firstSkip.Error)
		}
	}
	if issues > 1 {
		fmt.Fprintf(w, "  %s\n", p.Dim(fmt.Sprintf("%d more under Steps", issues-1)))
	}
}

// hint prints what to check next. The message itself is already under
// Steps, so it is not repeated.
func hint(w io.Writer, p ui.Palette, iss trace.Issue) {
	if iss.Hint != "" {
		fmt.Fprintf(w, "  %s  %s\n", p.Cyan("Hint"), iss.Hint)
	}
	if iss.Code != "" {
		fmt.Fprintf(w, "  %s  %s\n", p.Cyan("More"), "plumb explain "+iss.Code)
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// protoName turns IANA protocol numbers from the flow table into names.
func protoName(p string) string {
	switch p {
	case "1":
		return "icmp"
	case "6":
		return "tcp"
	case "17":
		return "udp"
	case "58":
		return "icmpv6"
	}
	return "proto " + p
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// Path writes one line per check, then a verdict that names the first check
// that blocks the traffic.
func Path(w io.Writer, p *trace.Path, o Options) {
	pal := ui.Palette{On: o.Color}
	from, to := trace.Name(p.From), trace.Name(p.To)
	traffic := "icmp echo"
	if p.Protocol != "icmp" {
		traffic = fmt.Sprintf("%s %d", p.Protocol, p.Port)
	}
	fmt.Fprintln(w, pal.Bold(fmt.Sprintf("Path %s → %s, %s", from, to, traffic)))
	var failed, skipped *trace.Check
	for i := range p.Checks {
		c := &p.Checks[i]
		detail := c.Detail
		switch {
		case c.Status == trace.StatusFail && failed == nil:
			failed = c
		case c.Status == trace.StatusSkip:
			detail = pal.Dim(detail)
			if skipped == nil {
				skipped = c
			}
		}
		fmt.Fprintf(w, "  %s %-9s %s\n", pal.Mark(string(c.Status)), c.Name, detail)
	}
	fmt.Fprintln(w)

	switch {
	case failed != nil:
		fmt.Fprintf(w, "%s %s\n", pal.Red("✗"), pal.Bold(fmt.Sprintf("%s cannot reach %s: the %s check fails", from, to, failed.Name)))
		hint(w, pal, trace.Issue{Code: failed.Code, Hint: failed.Hint})
	case skipped != nil:
		fmt.Fprintf(w, "%s %s\n", pal.Green("✓"), pal.Bold(fmt.Sprintf("Neutron allows %s to reach %s over %s", from, to, traffic)))
		fmt.Fprintf(w, "  %s %s: %s\n", pal.Dim("Skipped"), skipped.Name, skipped.Detail)
	default:
		fmt.Fprintf(w, "%s %s\n", pal.Green("✓"), pal.Bold(fmt.Sprintf("%s can reach %s over %s", from, to, traffic)))
	}
}
