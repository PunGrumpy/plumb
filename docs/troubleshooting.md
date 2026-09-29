<!-- contentType: Troubleshooting · plan: docs/content-plan.md -->

# What each warning means

This page explains every code that plumb shows in the `More  plumb explain <code>` line and in the `code` field of the JSON. Each section is named after a code and tells you what it means and what to check next. A warning means the API answered normally but the data doesn't match between layers. An error means the API call failed.

The `plumb explain <code>` command shows the same content in English, so you don't need to open this page.

## Credentials and connections

These codes can occur in any stage and in any check of `plumb doctor`.

### `no-credentials`

The shell has no `OS_*` variables yet. Source your openrc and run the command again. If you don't have an openrc yet, try `plumb demo`, which needs no credentials.

### `auth-failed`

Keystone answered HTTP 401. The username, password or project is wrong. Try `openstack token issue` with the same environment.

### `forbidden`

The token is valid, but its role can't read that object. Use a user with the `admin` or `reader` role in the project.

### `unreachable`

The machine that runs plumb can't open a connection to the endpoint. The error message gives the cause, such as `connection refused` or `no such host`. Run `plumb doctor` on the same machine to see which endpoints it reaches. If the machine can't reach the IP that the config records, use `--control-url` or `--agent-url`.

### `timeout`

The endpoint accepts the connection but doesn't answer within the time limit. Increase `--request-timeout` or check the load on that service.

### `no-recording`

The directory you pass to `--replay` has no file for this call. Record the lab again with `--record`, using the same set of flags.

## Stage `keystone`

If this stage fails, plumb skips the `nova` and `neutron` stages.

### `endpoint-missing`

The service catalog in the token has no endpoint for a service that plumb needs. Compare `OS_REGION_NAME` and `OS_INTERFACE` with the output of `openstack catalog list`.

### `no-admin`

The token has no `admin` role, so Nova doesn't show the VM's host. If the cloud uses OpenSDN, plumb can still find the compute node from the `virtual-router` in the Config API.

## Stages `nova` and `neutron`

The codes in these 2 stages relate to the project the token uses or the host the port is bound to.

### `vm-not-found`

No server with this UUID or name is visible to the token. A token without admin sees only its own project. A token with the `admin` role makes plumb search for the name in every project. Check the name with `openstack server list --all-projects`, or source the openrc of the project that owns the VM.

### `vm-ambiguous`

More than 1 server uses this name or IP. IPs can repeat when networks in different projects use the same subnet. The error message shows the UUID of every server. Run the command again with the UUID you want.

### `ip-not-found`

No port in Neutron uses this IP as a fixed IP, and no floating IP has this address. Check with `openstack port list --fixed-ip ip-address=your_ip` and `openstack floating ip list`. A token without admin sees only the ports of its own project.

### `ip-not-vm`

The IP exists, but it's on a port that doesn't belong to a VM, such as a router interface, a DHCP port or a floating IP that isn't bound to any port yet. The error message gives the port's `device_owner`. Trace the VM behind that device instead.

### `server-not-active`

Nova reports a server status other than `ACTIVE`. Run `openstack server show your_vm_name` and read the `fault` field.

### `lookup-failed`

plumb can't read 1 related object. The rest of the trace is still correct. Run the command again with `--debug` to see the failed call and its status code.

### `no-ports`

Neutron has no port whose `device_id` is this VM. Check with `openstack port list --server your_vm_name`. A VM with no port has no network.

### `binding-failed`

No Neutron mechanism driver can plug the port into the datapath on that host. Read the `neutron-server` log on the controller and check that the network agent on the compute node is still running.

### `port-not-active`

The port in Neutron isn't `ACTIVE` yet, which means the backend hasn't finished plugging the port. Check the network agent on the compute node the port is bound to.

### `host-mismatch`

Nova, Neutron and OpenSDN don't agree on the VM's compute node. Check for a stuck migration with `openstack server migration list`.

## Stage `opensdn-config`

The codes in this stage tell you whether Neutron and OpenSDN are in sync and whether the schema transformer has run.

### `vmi-missing`

The port exists in Neutron, but OpenSDN has no virtual machine interface (VMI) with the same UUID. There are 2 causes:

- This cloud doesn't use the OpenSDN plugin. If the port's `vif_type` isn't `vrouter`, run without `--config-url`.
- The 2 systems aren't in sync. Read the OpenSDN plugin log in `neutron-server`.

### `no-routing-instance`

The virtual network (VN) exists but has no routing instance (RI) yet, which means the schema transformer hasn't processed this VN. Check that the `contrail-schema` process is running and has no errors in its log.

### `no-route-target`

The RI has no route target (RT), so other VRFs can't import this RI's routes. Check `contrail-schema` the same way as for `no-routing-instance`.

### `compute-unknown`

plumb can't follow `virtual-machine` to `virtual-router`, so it doesn't know the vRouter agent's IP. Use `--agent-url` to point to the introspect of the compute node the VM runs on.

### `control-discovery`

plumb can't read the `bgp-router` list to find the control nodes. Use `--control-url`.

## Stage `control`

The codes in this stage tell you whether the agent has announced the VM's route to the control node.

### `control-unreachable`

plumb can't read the control node's introspect on port 8083. Run `plumb doctor`, or use `--control-url` to point to an address this machine can reach.

### `xmpp-missing`

The control node has no Extensible Messaging and Presence Protocol (XMPP) session in the `Established` state with the agent on the VM's compute node. Look at the `XMPP` line under `Compute`, which shows the session from the agent's side.

### `route-missing`

The session may be fine, but the VM's route isn't in the RI's table on the control node. Check these 3 things in order:

1. The port's `vRouter` line must show `✓ active`.
2. The agent may connect only to another control node. Check that the BGP session between the control nodes is in the `Established` state.
3. The RI name in the Config API must match the VRF name on the agent.

## Stage `vrouter`

The codes in this stage tell you the state on the VM's compute node and check that the label matches what the control node announces.

### `agent-xmpp-down`

The agent has no `Established` XMPP session with any control node, so the control nodes get no routes from this compute node. Check the network from the compute node to the control nodes on TCP port 5269.

### `interface-missing`

The agent doesn't know this port. Nova may not have plugged the tap yet, or the agent hasn't received the VMI config yet. Read the `nova-compute` log.

### `interface-inactive`

The agent knows the interface but hasn't activated it yet. Check that the agent has received the config for the VN and the VM's IP.

### `label-mismatch`

The label the control node announces doesn't match the label the agent uses, so other compute nodes send packets with a label the agent doesn't know. One cause is that the control node still holds an old route. Open `Snh_ShowRouteReq` for that prefix in the control introspect and check when the route last changed.

### `agent-route-missing`

The VRF on the agent has no route for the VM's IP. Check that the IP in Neutron matches the IP the VM uses.

## The `plumb path` command

These codes come from checking the path between 2 VMs. The `Hint` line of `plumb path` gives the command that fixes the problem on that path.

### `path-no-router`

The two VMs are on different subnets, and no Neutron router has an interface on both networks. Connect both subnets to the same router with `openstack router add subnet`, or reach the destination VM through a floating IP.

### `sg-egress-blocked`

No egress rule in the source VM's security groups allows this traffic to the destination. The default OpenStack security group allows all egress, so this code means someone removed that rule or the port uses a different security group.

### `sg-ingress-blocked`

No ingress rule in the destination VM's security groups allows this traffic from the source VM. The `default` security group allows ingress only from ports in the same group, so a source VM in a different group is blocked. Add a rule with the command in the `Hint` line, or use `--remote-group` instead of `--remote-ip` if you want to allow the whole group.

### `path-no-route`

The source VM's VRF on its compute node has no route to the destination IP. If the VMs are in different VNs, the source's routing instance must import the destination's route target. Check the network policy or logical router between the 2 VNs. If they're in the same VN, run `plumb trace` on the destination to see whether the control node has its route.

### `path-wrong-next-hop`

The source VRF has a route, but the route doesn't go to the compute node or interface the destination VM is on. One cause is that the control node still holds an old route after the VM moved to another compute node. Run `plumb trace` on the destination and compare the `Control` line with the current compute node.
