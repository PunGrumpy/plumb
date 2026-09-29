package trace

import "fmt"

// Trace is everything plumb learned about one VM. It is the only contract
// between the stages and the renderers: the tree, --json and any future web
// UI all read this struct and nothing else.
type Trace struct {
	Query        string        `json:"query"` // what the user typed: UUID or name
	VMID         string        `json:"vm_id"`
	Steps        []Step        `json:"steps"`
	Identity     *Identity     `json:"identity,omitempty"`
	Server       *Server       `json:"server,omitempty"`
	Compute      *Compute      `json:"compute,omitempty"`
	ControlNodes []ControlNode `json:"control_nodes,omitempty"`
	Ports        []*Port       `json:"ports"`
}

type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Step records how one stage went. Warnings mean the API calls worked but
// the chain is broken or inconsistent somewhere.
type Step struct {
	Name       string  `json:"name"`
	Status     Status  `json:"status"`
	Error      string  `json:"error,omitempty"`
	Code       string  `json:"code,omitempty"` // set when Status is fail
	Hint       string  `json:"hint,omitempty"`
	Warnings   []Issue `json:"warnings,omitempty"`
	DurationMS int64   `json:"duration_ms"`
}

// Failed reports whether any stage failed outright.
func (t *Trace) Failed() bool {
	for _, s := range t.Steps {
		if s.Status == StatusFail {
			return true
		}
	}
	return false
}

type Identity struct {
	User      string            `json:"user"`
	Project   string            `json:"project"`
	Roles     []string          `json:"roles"`
	Admin     bool              `json:"admin"`
	Endpoints map[string]string `json:"endpoints"`
}

type Server struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Host         string `json:"host,omitempty"`
	InstanceName string `json:"instance_name,omitempty"`
	Flavor       Flavor `json:"flavor"`
	ImageID      string `json:"image_id,omitempty"`
	ImageName    string `json:"image_name,omitempty"`
}

type Flavor struct {
	Name   string `json:"name"`
	VCPUs  int    `json:"vcpus"`
	RAMMB  int    `json:"ram_mb"`
	DiskGB int    `json:"disk_gb"`
}

// Compute is the hypervisor as OpenSDN knows it: a virtual-router object
// in config and a vRouter agent at IP.
type Compute struct {
	Name     string        `json:"name,omitempty"`
	IP       string        `json:"ip,omitempty"`
	AgentURL string        `json:"agent_url,omitempty"`
	XMPP     []XMPPSession `json:"xmpp,omitempty"`
}

type XMPPSession struct {
	Controller string `json:"controller"`
	State      string `json:"state"`
	Config     bool   `json:"config"`
}

type ControlNode struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address,omitempty"`
	URL     string `json:"url"`
	// XMPPState is this node's view of its session with the VM's compute.
	XMPPState string `json:"xmpp_state,omitempty"`
}

// Port follows one VM interface through every layer. The same UUID is the
// Nova interface, the Neutron port and the OpenSDN VMI.
type Port struct {
	ID      string         `json:"id"`
	MAC     string         `json:"mac,omitempty"`
	IPs     []string       `json:"ips,omitempty"`
	Neutron *NeutronPort   `json:"neutron,omitempty"`
	Config  *ConfigChain   `json:"config,omitempty"`
	Control []ControlRoute `json:"control,omitempty"`
	VRouter *VRouterView   `json:"vrouter,omitempty"`
}

type NeutronPort struct {
	Status         string          `json:"status"`
	VIFType        string          `json:"vif_type"`
	VNICType       string          `json:"vnic_type"`
	BindingHost    string          `json:"binding_host"`
	PortSecurity   bool            `json:"port_security"`
	Network        *Network        `json:"network,omitempty"`
	Subnets        []Subnet        `json:"subnets,omitempty"`
	SecurityGroups []SecurityGroup `json:"security_groups,omitempty"`
	FloatingIPs    []string        `json:"floating_ips,omitempty"`
}

type Network struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Type           string   `json:"type,omitempty"`
	SegmentationID *int     `json:"segmentation_id,omitempty"`
	MTU            int      `json:"mtu,omitempty"`
	Routers        []Router `json:"routers,omitempty"`
}

// Router is a Neutron router with an interface on the network.
type Router struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Subnet struct {
	ID      string `json:"id"`
	CIDR    string `json:"cidr"`
	Gateway string `json:"gateway,omitempty"`
}

type SecurityGroup struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// Rule is one security group rule. Empty Protocol means any protocol and
// nil ports mean any port. For ICMP, PortMin is the type and PortMax the
// code; type 0 (echo reply) is a real value, hence the pointers.
type Rule struct {
	Direction      string `json:"direction"`
	EtherType      string `json:"ethertype"`
	Protocol       string `json:"protocol,omitempty"`
	PortMin        *int   `json:"port_min,omitempty"`
	PortMax        *int   `json:"port_max,omitempty"`
	RemoteIPPrefix string `json:"remote_ip_prefix,omitempty"`
	RemoteGroupID  string `json:"remote_group_id,omitempty"`
}

// String renders the rule as "ingress IPv4 tcp 22 from 0.0.0.0/0".
func (r Rule) String() string {
	s := fmt.Sprintf("%s %s %s", r.Direction, r.EtherType, or(r.Protocol, "any"))
	switch {
	case r.PortMin != nil && r.PortMax != nil && *r.PortMin != *r.PortMax:
		s += fmt.Sprintf(" %d-%d", *r.PortMin, *r.PortMax)
	case r.PortMin != nil:
		s += fmt.Sprintf(" %d", *r.PortMin)
	}
	peer := "any"
	switch {
	case r.RemoteIPPrefix != "":
		peer = r.RemoteIPPrefix
	case r.RemoteGroupID != "":
		peer = "group " + r.RemoteGroupID
	}
	if r.Direction == "egress" {
		return s + " to " + peer
	}
	return s + " from " + peer
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// ConfigChain is the OpenSDN object graph behind one port:
// VMI → virtual-network → routing-instance → route-targets.
type ConfigChain struct {
	VMI             Object        `json:"vmi"`
	VirtualNetwork  *VN           `json:"virtual_network,omitempty"`
	RoutingInstance *Object       `json:"routing_instance,omitempty"`
	RouteTargets    []RouteTarget `json:"route_targets,omitempty"`
	InstanceIPs     []string      `json:"instance_ips,omitempty"`
	FloatingIPs     []string      `json:"floating_ips,omitempty"`
	SecurityGroups  []string      `json:"security_groups,omitempty"`
}

type Object struct {
	UUID   string `json:"uuid"`
	FQName string `json:"fq_name"`
}

type VN struct {
	Object
	NetworkID        int      `json:"network_id"`
	VxlanID          int      `json:"vxlan_id,omitempty"`
	ForwardingMode   string   `json:"forwarding_mode,omitempty"`
	UserRouteTargets []string `json:"user_route_targets,omitempty"`
	Policies         []string `json:"policies,omitempty"`
}

type RouteTarget struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

// ControlRoute is what one control node holds for one VM prefix.
type ControlRoute struct {
	ControlNode string        `json:"control_node"`
	Table       string        `json:"table"`
	Prefix      string        `json:"prefix"`
	Found       bool          `json:"found"`
	Paths       []ControlPath `json:"paths,omitempty"`
}

type ControlPath struct {
	Protocol string   `json:"protocol"`
	Source   string   `json:"source,omitempty"`
	NextHop  string   `json:"next_hop"`
	Label    string   `json:"label"`
	Encap    []string `json:"encap,omitempty"`
	OriginVN string   `json:"origin_vn,omitempty"`
}

type VRouterView struct {
	Interface    *AgentInterface `json:"interface,omitempty"`
	VRFIndex     int             `json:"vrf_index,omitempty"`
	Routes       []AgentRoute    `json:"routes,omitempty"`
	Flows        []Flow          `json:"flows,omitempty"`
	FlowsSampled int             `json:"flows_sampled"`
}

type AgentInterface struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	VRF    string `json:"vrf"`
	Label  string `json:"label"`
}

type AgentRoute struct {
	Prefix string      `json:"prefix"`
	Paths  []AgentPath `json:"paths"`
}

type AgentPath struct {
	Peer       string `json:"peer"`
	NHType     string `json:"nh_type"`
	Interface  string `json:"interface,omitempty"`
	TunnelDst  string `json:"tunnel_dst,omitempty"`
	TunnelType string `json:"tunnel_type,omitempty"`
	Label      string `json:"label"`
}

type Flow struct {
	Src        string `json:"src"`
	Dst        string `json:"dst"`
	Protocol   string `json:"protocol"`
	DropReason string `json:"drop_reason,omitempty"`
}
