package trace

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"plumb/internal/httpx"
)

// Issue is one problem a stage found. Code is stable: it is the key for
// `plumb explain` and the anchor in docs/troubleshooting.md.
type Issue struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// Explanation says what an issue code means and what to check next.
type Explanation struct {
	Code    string
	Meaning string
	Check   string
}

var explanations = []Explanation{
	{"no-credentials", "The OS_* variables that openrc exports are missing.",
		"Run `source openrc admin admin`, or try the offline lab with `plumb demo`."},
	{"auth-failed", "Keystone rejected the credentials.",
		"Check OS_USERNAME, OS_PASSWORD and OS_PROJECT_NAME. `openstack token issue` must work with the same environment."},
	{"forbidden", "The token is valid but its role may not read this object.",
		"Use a user with the admin or reader role on this project."},
	{"unreachable", "This machine cannot open a connection to the endpoint.",
		"Run `plumb doctor` on this machine to see which endpoints it reaches, or pass --control-url and --agent-url with addresses it can reach."},
	{"timeout", "The endpoint accepted the connection but did not answer in time.",
		"Raise --request-timeout, or check the load on the service."},
	{"no-recording", "A --replay directory has no response for this call.",
		"Record the lab again with --record, using the same flags."},
	{"endpoint-missing", "The service catalog in the token has no endpoint for a service plumb needs.",
		"Check OS_REGION_NAME and OS_INTERFACE against `openstack catalog list`."},
	{"no-admin", "Nova shows the VM's host only to the admin role.",
		"Use an admin user to see the host. On OpenSDN, plumb still finds the compute through its virtual-router."},
	{"vm-not-found", "No server with this UUID or name is visible to the token. A non-admin token sees only its own project.",
		"Check the name with `openstack server list --all-projects`, or source the openrc of the project that owns the VM."},
	{"vm-ambiguous", "More than one server has this name or IP.",
		"Pass the UUID from the list in the error instead."},
	{"ip-not-found", "No Neutron port has this fixed IP, and no floating IP has this address.",
		"Check the address with `openstack port list --fixed-ip ip-address=<ip>` and `openstack floating ip list`."},
	{"ip-not-vm", "The IP exists, but on a port that no VM owns, such as a router interface, a DHCP port or an unassociated floating IP.",
		"The error names the port's device_owner. Trace the VM behind that device instead."},
	{"server-not-active", "Nova reports the server in a state other than ACTIVE.",
		"Run `openstack server show <vm>` and read the fault field."},
	{"lookup-failed", "plumb could not read one related object. The rest of the trace is still valid.",
		"Rerun with --dump-http to see the failing call and its status code."},
	{"no-ports", "Neutron has no port whose device_id is this VM.",
		"Run `openstack port list --server <vm>`. A VM without a port has no network."},
	{"binding-failed", "No Neutron mechanism driver could plug the port into a datapath on the host.",
		"Read the neutron-server log on the controller, and check that the network agent on the compute runs."},
	{"port-not-active", "The Neutron port is not ACTIVE, so the backend has not finished plugging it.",
		"Check the network agent on the compute that the port is bound to."},
	{"host-mismatch", "Nova, Neutron and OpenSDN disagree on which compute runs the VM.",
		"Look for an unfinished migration with `openstack server migration list`."},
	{"vmi-missing", "The Neutron port has no virtual-machine-interface with the same UUID in OpenSDN.",
		"If the port's vif_type is not vrouter, rerun without --config-url. Otherwise read the OpenSDN plugin log in neutron-server."},
	{"no-routing-instance", "The virtual network has no routing instance, so the schema transformer has not processed it.",
		"Check that the contrail-schema process runs and has no errors in its log."},
	{"no-route-target", "The routing instance has no route target, so no other VRF can import its routes.",
		"Check that the contrail-schema process runs and has no errors in its log."},
	{"compute-unknown", "plumb could not follow virtual-machine to virtual-router, so it does not know the agent's IP.",
		"Pass --agent-url with the vRouter agent introspect URL of the VM's compute."},
	{"control-discovery", "plumb could not list bgp-routers to find the control nodes.",
		"Pass --control-url with the control node introspect URLs."},
	{"control-unreachable", "plumb could not read a control node's introspect on port 8083.",
		"Run `plumb doctor`, or pass --control-url with an address this machine reaches."},
	{"xmpp-missing", "A control node has no Established XMPP session with the VM's compute.",
		"Compare with the XMPP lines under Compute, which show the agent's side of the session."},
	{"route-missing", "A control node has no route for the VM's IP in the routing instance's table.",
		"Check that the port's vRouter line shows active, then check the agent's XMPP sessions."},
	{"agent-xmpp-down", "The vRouter agent has no Established XMPP session, so no control node learns its routes.",
		"Check connectivity from the compute to the control nodes on TCP port 5269."},
	{"interface-missing", "The vRouter agent does not know the port.",
		"Nova may not have plugged the tap yet, or the agent has no config for the VMI. Check the nova-compute log."},
	{"interface-inactive", "The vRouter agent knows the interface but has not activated it.",
		"Check that the agent received the virtual network config and the VM's IP."},
	{"label-mismatch", "A control node advertises the VM's route with a label the agent no longer uses, so remote computes send packets the agent cannot deliver.",
		"Open Snh_ShowRouteReq for the prefix on that control node and check when the route last changed."},
	{"path-no-router", "The two VMs are on different subnets, and no Neutron router has an interface on both networks.",
		"Attach both subnets to one router with `openstack router add subnet`, or reach the VM through its floating IP."},
	{"sg-egress-blocked", "No egress rule in the source VM's security groups allows this traffic to the destination.",
		"Add an egress rule, or check that the right security group is on the port."},
	{"sg-ingress-blocked", "No ingress rule in the destination VM's security groups allows this traffic from the source.",
		"Add an ingress rule for the protocol and port, with the source IP or the source's security group as remote."},
	{"path-no-route", "The source VM's VRF on its compute has no route to the destination IP.",
		"The source routing instance must import the destination's route target. Check the network policy or router between the networks."},
	{"path-wrong-next-hop", "The source VRF has a route, but it does not lead to the compute or interface where the destination VM runs.",
		"The control nodes may hold a stale route. Trace the destination and compare its Control lines."},
	{"agent-route-missing", "The VRF on the agent has no route for the VM's IP.",
		"Check that the IP in Neutron is the IP the VM uses."},
}

// Explain returns the explanation for code.
func Explain(code string) (Explanation, bool) {
	for _, e := range explanations {
		if e.Code == code {
			return e, true
		}
	}
	return Explanation{}, false
}

// Codes lists every issue code in alphabetical order.
func Codes() []string {
	var out []string
	for _, e := range explanations {
		out = append(out, e.Code)
	}
	sort.Strings(out)
	return out
}

func newIssue(code, msg string) Issue {
	e, _ := Explain(code)
	return Issue{Code: code, Message: msg, Hint: e.Check}
}

// Classify turns a stage error into an Issue with a code when the cause is
// recognisable.
func Classify(err error) Issue {
	msg := err.Error()
	var ce *codedError
	if errors.As(err, &ce) {
		return newIssue(ce.code, msg)
	}
	var se *httpx.StatusError
	if errors.As(err, &se) {
		switch se.Code {
		case http.StatusUnauthorized:
			return newIssue("auth-failed", msg)
		case http.StatusForbidden:
			return newIssue("forbidden", msg)
		}
	}
	if strings.Contains(msg, "replay: no recording") {
		return newIssue("no-recording", msg)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newIssue("timeout", msg)
	}
	var ue *url.Error
	if errors.As(err, &ue) && ue.Timeout() {
		return newIssue("timeout", msg)
	}
	var oe *net.OpError
	var de *net.DNSError
	if errors.As(err, &oe) || errors.As(err, &de) {
		return newIssue("unreachable", msg)
	}
	return Issue{Message: msg}
}
