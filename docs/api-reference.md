<!-- contentType: Reference · plan: docs/content-plan.md -->

# APIs plumb calls

This page lists every HTTP call that plumb makes, grouped by stage and in the order they appear in `apps/cli/internal/trace`. Each call lists its parameters, the fields plumb reads and what plumb uses each field for. The files in `apps/cli/internal/demo/lab` follow the formats on this page. They aren't recorded from an OpenSDN lab.

## Field stability

The API field names in the first 4 stages don't change between releases. The Sandesh introspect in the `control` and `vrouter` stages has no version, so request and field names can change with each OpenSDN release. To compare them with your lab, see [Use plumb with DevStack and OpenSDN](run-against-a-lab.md).

## Keystone

The `keystone` stage requests a project-scoped token and reads the endpoints from the service catalog.

| Call | Fields read | What plumb uses it for |
| --- | --- | --- |
| `POST /v3/auth/tokens` | header `X-Subject-Token` | Token for the next calls |
|  | `token.roles[].name` | Check for the `admin` role |
|  | `token.catalog[]` | Endpoints of `compute`, `network` and `image` |

## Nova and Glance

The `nova` stage reads the server with microversion 2.47, which embeds the flavor in the server.

| Call | Fields read | What plumb uses it for |
| --- | --- | --- |
| `GET /servers?name=^{vm_name}$` | `id` | The VM's UUID when you pass a name. Nova treats `name` as a regular expression, so plumb escapes the name and wraps it in `^` and `$` |
| Neutron `GET /v2.0/ports?fixed_ips=ip_address={ip}` | `device_id`, `device_owner` | The VM that uses the fixed IP when you pass an IP |
| `GET /v2.0/floatingips?floating_ip_address={ip}` then `GET /v2.0/ports/{port_id}` | `port_id`, `device_id` | The VM that uses the floating IP when no fixed IP matches |
| `GET /servers/{vm_id}` | `OS-EXT-SRV-ATTR:host` | The VM's host. Needs the `admin` role |
|  | `flavor.original_name`, `vcpus`, `ram`, `disk` | The VM's size |
|  | `image.id` | Empty when the VM boots from a volume |
| `GET /servers/{vm_id}/os-interface` | `port_id`, `mac_addr`, `fixed_ips` | Ports that Nova attaches |
| Glance `GET /v2/images/{image_id}` | `name` | The image name |

## Neutron

The `neutron` stage finds ports by `device_id`, then reads the objects around each port.

| Call | Fields read | What plumb uses it for |
| --- | --- | --- |
| `GET /v2.0/ports?device_id={vm_id}` | `binding:vif_type` | The backend that plugs the port into the datapath |
|  | `binding:host_id` | Compare with Nova's host |
|  | `fixed_ips`, `security_groups` | Keys for the next calls |
| `GET /v2.0/networks/{id}` | `provider:network_type`, `provider:segmentation_id` | Network type and segment number |
| `GET /v2.0/subnets/{id}` | `cidr`, `gateway_ip` | Show the subnet and gateway |
| `GET /v2.0/security-groups/{id}` | `security_group_rules[]` | Turn each rule into 1 line of text |
| `GET /v2.0/floatingips?port_id={id}` | `floating_ip_address` | Show the port's floating IP |

## OpenSDN Config API

The `opensdn-config` stage reads objects with `GET /{type}/{uuid}`, which returns `{"{type}": {…}}`.

This table uses 4 abbreviations: VMI for virtual machine interface, VN for virtual network, RI for routing instance and RT for route target. Every ref has a `to` field that already holds the target's `fq_name`, so plumb doesn't call `GET /route-target/{uuid}`.

| Call | Fields read | What plumb uses it for |
| --- | --- | --- |
| `GET /virtual-machine-interface/{port_id}` | `virtual_network_refs` | The port's VN |
|  | `routing_instance_refs` | The port's RI |
|  | `instance_ip_back_refs`, `floating_ip_back_refs` | IPs that the config reserves |
| `GET /virtual-network/{uuid}` | `virtual_network_network_id` | The VN's internal ID |
|  | `virtual_network_properties` | VXLAN network identifier (VNI) and forwarding mode |
|  | `route_target_list`, `routing_instances` | User-defined RTs and fallback RIs |
| `GET /routing-instance/{uuid}` | `route_target_refs[].to`, `attr.import_export` | RTs and their direction |
| `GET /virtual-machine/{vm_id}` | `virtual_router_back_refs` | The compute node the VM runs on |
| `GET /virtual-router/{uuid}` | `virtual_router_ip_address` | The agent's IP |
| `GET /bgp-routers?detail=true` | `bgp_router_parameters.router_type`, `address` | List of control nodes |
| `GET /virtual-routers?detail=true` | `fq_name`, `virtual_router_ip_address` | List of compute nodes that `plumb doctor` probes |

## Control node introspect

The `control` stage calls every control node on port 8083. Field names differ between releases.

| Call | Elements read | What plumb uses it for |
| --- | --- | --- |
| `GET /Snh_ShowBgpNeighborSummaryReq` | `BgpNeighborResp`: `peer`, `peer_address`, `encoding`, `state` | Extensible Messaging and Presence Protocol (XMPP) sessions with compute nodes |
| `GET /Snh_ShowRouteReq?routing_table={ri}.inet.0&prefix={ip}/32` | `ShowRoutePath` in `ShowRoute` in `ShowRouteTable` | The VM's routes |

plumb reads these fields of `ShowRoutePath`:

- `protocol`: the value `XMPP` means the route came from the agent
- `source`: the name of the peer that sent the route
- `next_hop`: the IP of the compute node the VM runs on
- `label`: the interface's Multiprotocol Label Switching (MPLS) label
- `tunnel_encap`: the overlays you can use, such as `gre` or `udp`
- `origin_vn`: the route's source VN

## vRouter agent introspect

The `vrouter` stage calls the agent on the compute node on port 8085. Field names differ between releases.

| Call | Elements read | What plumb uses it for |
| --- | --- | --- |
| `GET /Snh_AgentXmppConnectionStatusReq` | `AgentXmppData`: `controller_ip`, `state`, `cfg_controller` | The control nodes the agent connects to |
| `GET /Snh_ItfReq?uuid={port_id}` | `ItfSandeshData`: `name`, `vrf_name`, `active`, `label` | The tap interface |
| `GET /Snh_VrfListReq?name={vrf}` | `VrfSandeshData`: `ucindex` | The index used to look up routes |
| `GET /Snh_Inet4UcRouteReq?vrf_index={n}&src_ip={ip}&prefix_len=32` | `NhSandeshData` in `PathSandeshData` in `RouteUcSandeshData` | The VM's next hop |
| `GET /Snh_FetchAllFlowRecords` | `SandeshFlowData`: `sip`, `dip`, `src_port`, `dst_port`, `protocol`, `drop_reason` | The VM's flows |

`FetchAllFlowRecords` returns only the first page of the flow table, so the flow count in the tree is a sample, not the total.

The `type` field of `NhSandeshData` gives the next hop type:

- `interface`: the VM is on this compute node
- `tunnel`: the packet goes to another compute node. `dip` is the destination IP and `tunnel_type` is the overlay type
