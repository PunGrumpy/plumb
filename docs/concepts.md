<!-- contentType: Conceptual · plan: docs/content-plan.md -->

# How a VM's request crosses each layer

This page explains which components a VM on OpenStack with OpenSDN goes through before it can send packets, and which values each component passes to the next. Each section points to the line in plumb's tree that confirms it.

This page calls the sequence of objects that links a VM to its route on the vRouter agent the chain. It uses 4 OpenSDN object names: a virtual machine interface (VMI) is the OpenSDN side of a port, a virtual network (VN) is a network, a routing instance (RI) is the routing table of a VN, and a route target (RT) is a tag that says which routes an RI can import.

## The chain at a glance

When you create a VM, data flows from the OpenStack API down to the kernel module on the compute node in this order:

```text
openstack server create
  → Nova         picks a compute node and asks Neutron for a port
  → Neutron      hands off to the OpenSDN plugin
  → Config API   stores the VMI and VN
  → Schema transformer  creates the RI and RT
  → Control node sends config to the agent over XMPP
  → vRouter agent  creates the tap, VRF and routes
  → Control node receives the VM's route and spreads it
```

plumb reads the chain in the same direction. Each layer uses a value from the layer before it as the key:

| From layer | Value passed on | To layer |
| --- | --- | --- |
| Keystone | service catalog | Nova and Neutron endpoints |
| Nova | VM UUID | port with the same `device_id` |
| Neutron | port UUID | VMI with the same UUID |
| Config | the VMI's `routing_instance_refs` | route table name on the control node |
| Config | the VM's `virtual_router_back_refs` | IP of the vRouter agent |
| Agent | port UUID | tap interface and VRF |

## Neutron doesn't wire the network itself

Neutron accepts the network, subnet and port APIs and hands them to the backend's core plugin. On OpenSDN, the plugin turns every call into an object in the Config API and keeps the same UUID. So a port becomes a VMI with the same UUID.

A port's `binding:vif_type` says which backend plugs the port into the datapath. On OpenSDN, this value is `vrouter`. When Nova creates a VM, it reads this value and plugs the tap interface into the vRouter instead of Open vSwitch.

plumb's tree shows this in 2 lines:

- `Neutron  ACTIVE  vif_type=vrouter`
- `Config  VMI …  ✓ same UUID as the port`

## Config is separate from the control node because intent differs from routing

The Config API stores what the user wants, such as a VN named `vn1` attached to a policy. The control node needs routing data, such as RIs and RTs, which the user doesn't create.

The schema transformer is a process that reads intent from the Config API and creates the routing objects:

- The schema transformer creates 1 RI per VN. The RI's `fq_name` is the VN's `fq_name` with the VN name appended again, such as `default-domain:admin:vn1:vn1`.
- The schema transformer creates an RT for the RI automatically. The number comes from a range the schema transformer reserves, such as `target:64512:8000002` in the built-in lab.
- When a policy allows 2 VNs to talk, the schema transformer makes each side's RI import the other side's RT.

Splitting the 2 layers lets users change intent without knowing about routing. I think this benefit is worth it, but it has a cost: the schema transformer is one more process that can stop while the Config API still responds normally. If a VN exists but has no RI, the schema transformer hasn't processed that VN yet. So plumb checks the RI separately from the VN and warns in the `opensdn-config` stage.

## XMPP carries data between the control node and the agent

The Extensible Messaging and Presence Protocol (XMPP) is the channel between the control node and the vRouter agent. Data flows both ways:

- The control node sends the agent the config for the VMIs, VNs and RIs it needs.
- The agent sends the control node the routes of the VMs on its compute node, with the compute node's IP as the next hop and the interface's label.

An agent connects to at most 2 control nodes. If one goes down, the agent still sends and receives routes through the other. The agent picks 1 control node as its config source, and the tree shows `config` at the end of that control node's `XMPP` line.

The line `path  XMPP from compute-02  nh 10.10.0.21  label 25` in the tree means the control node got the VM's route from the agent on `compute-02`. If the route is missing, the problem is between the agent and the control node, not in the Config API.

## BGP carries routes between control nodes and to the gateway

Control nodes talk to each other and to the gateway router with Border Gateway Protocol (BGP) in layer 3 VPN (L3VPN) mode. This is the same mode network providers use to keep customer VPNs apart. This has 2 results:

- Every control node sees the same set of routes, even when agents connect to different control nodes.
- The gateway router can take the VM's routes and use them, such as when the VM uses a floating IP.

plumb reads the peer list from `Snh_ShowBgpNeighborSummaryReq`, which lists both BGP and XMPP peers together, and tells them apart by the `encoding` field.

## VRFs keep tenants apart

A virtual routing and forwarding (VRF) table is a separate routing table per RI on the agent. Every VM in the same VN is in the same VRF, and a VRF holds only the routes whose RT matches an RT that the VRF's RI imports.

So 2 tenants that both use the subnet `10.0.1.0/24` don't collide, because each tenant's routes are in a different VRF and have a different RT.

In the tree, the RI name on the `Config` line, the table name on the `Control` line and the `vrf` name on the `vRouter` line all come from the same name, `default-domain:admin:vn1:vn1`.

## The overlay is in the encap and the label

For packets between VMs on different compute nodes, the vRouter wraps each packet in an overlay header and sends it across the underlay to the destination compute node's IP. OpenSDN supports 3 types:

- MPLSoUDP puts a Multiprotocol Label Switching (MPLS) label inside UDP. The tree shows it as `udp`.
- MPLSoGRE puts an MPLS label inside Generic Routing Encapsulation (GRE). The tree shows it as `gre`.
- VXLAN, or Virtual Extensible LAN, uses the VN's VXLAN network identifier instead of a label. It's used for layer 2 EVPN routes. The tree shows it as `vxlan`.

The label tells the destination compute node which interface to send the packet to, so the label must match at every layer. plumb compares the label the control node advertises with the label the agent assigns to the tap interface, and warns when they don't match.

Routes to VMs on the same compute node show the next hop as `local interface`. Routes to VMs on another compute node show `tunnel MPLSoUDP to` followed by that compute node's IP.
