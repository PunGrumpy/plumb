<!-- contentType: Reference · plan: docs/content-plan.md -->

# API ที่ plumb เรียกในแต่ละชั้น

หน้านี้รวม HTTP call ทุกตัวที่ plumb เรียก เรียงตาม stage และตามลำดับใน `internal/trace` แต่ละ call มี parameter, field ที่อ่าน และสิ่งที่ plumb ใช้ field นั้นทำ ไฟล์ใน `internal/demo/lab` เขียนตามรูปแบบในหน้านี้ ไม่ได้บันทึกจาก lab OpenSDN

## ความคงที่ของ field

ชื่อ field ของ API ใน 4 stage แรกไม่เปลี่ยนระหว่าง release ส่วน introspect ของ Sandesh ใน stage `control` และ `vrouter` ไม่มี version กำกับ ชื่อ request และ field จึงเปลี่ยนได้ตาม release ของ OpenSDN วิธีเทียบกับ lab อยู่ใน [วิธีใช้ plumb กับ DevStack และ lab OpenSDN](run-against-a-lab.md)

## Keystone

Stage `keystone` ขอ token แบบผูก project และอ่าน endpoint จาก service catalog

| Call | Field ที่อ่าน | ใช้ทำอะไร |
| --- | --- | --- |
| `POST /v3/auth/tokens` | header `X-Subject-Token` | token ของ call ถัดไป |
| | `token.roles[].name` | ตรวจ role `admin` |
| | `token.catalog[]` | endpoint ของ `compute`, `network` และ `image` |

## Nova และ Glance

Stage `nova` อ่าน server ด้วย microversion 2.47 ซึ่งแนบ flavor มาใน server

| Call | Field ที่อ่าน | ใช้ทำอะไร |
| --- | --- | --- |
| `GET /servers?name=^{vm_name}$` | `id` | UUID ของ VM เมื่อผู้ใช้ส่งชื่อ Nova ตีความ `name` เป็น regular expression plumb จึง escape ชื่อและครอบด้วย `^` กับ `$` |
| `GET /v2.0/ports?fixed_ips=ip_address={ip}` ของ Neutron | `device_id`, `device_owner` | VM ที่ใช้ fixed IP เมื่อผู้ใช้ส่ง IP |
| `GET /v2.0/floatingips?floating_ip_address={ip}` แล้ว `GET /v2.0/ports/{port_id}` | `port_id`, `device_id` | VM ที่ใช้ floating IP เมื่อไม่เจอ fixed IP |
| `GET /servers/{vm_id}` | `OS-EXT-SRV-ATTR:host` | host ของ VM ต้องใช้ role `admin` |
| | `flavor.original_name`, `vcpus`, `ram`, `disk` | ขนาดของ VM |
| | `image.id` | ค่าว่างเมื่อ boot จาก volume |
| `GET /servers/{vm_id}/os-interface` | `port_id`, `mac_addr`, `fixed_ips` | port ที่ Nova attach |
| `GET /v2/images/{image_id}` ของ Glance | `name` | ชื่อ image |

## Neutron

Stage `neutron` ค้น port ด้วย `device_id` แล้วอ่าน object รอบ port

| Call | Field ที่อ่าน | ใช้ทำอะไร |
| --- | --- | --- |
| `GET /v2.0/ports?device_id={vm_id}` | `binding:vif_type` | backend ที่เสียบ port เข้า datapath |
| | `binding:host_id` | เทียบกับ host ของ Nova |
| | `fixed_ips`, `security_groups` | key ของ call ถัดไป |
| `GET /v2.0/networks/{id}` | `provider:network_type`, `provider:segmentation_id` | ชนิดและเลข segment |
| `GET /v2.0/subnets/{id}` | `cidr`, `gateway_ip` | แสดง subnet และ gateway |
| `GET /v2.0/security-groups/{id}` | `security_group_rules[]` | แปลง rule เป็นข้อความ 1 บรรทัด |
| `GET /v2.0/floatingips?port_id={id}` | `floating_ip_address` | แสดง floating IP ของ port |

## OpenSDN Config API

Stage `opensdn-config` อ่าน object ด้วย `GET /{type}/{uuid}` ซึ่งตอบกลับเป็น `{"{type}": {…}}`

ตารางนี้ใช้ชื่อย่อ 4 ตัว คือ VMI สำหรับ virtual machine interface, VN สำหรับ virtual network, RI สำหรับ routing instance และ RT สำหรับ route target Ref ทุกตัวมี `to` ซึ่งเป็น `fq_name` ของปลายทางอยู่แล้ว plumb จึงไม่เรียก `GET /route-target/{uuid}`

| Call | Field ที่อ่าน | ใช้ทำอะไร |
| --- | --- | --- |
| `GET /virtual-machine-interface/{port_id}` | `virtual_network_refs` | VN ของ port |
| | `routing_instance_refs` | RI ของ port |
| | `instance_ip_back_refs`, `floating_ip_back_refs` | IP ที่ config จองไว้ |
| `GET /virtual-network/{uuid}` | `virtual_network_network_id` | ID ภายในของ VN |
| | `virtual_network_properties` | VXLAN network identifier (VNI) และ forwarding mode |
| | `route_target_list`, `routing_instances` | RT ที่ผู้ใช้กำหนดเอง และ RI สำรอง |
| `GET /routing-instance/{uuid}` | `route_target_refs[].to`, `attr.import_export` | RT และทิศทาง |
| `GET /virtual-machine/{vm_id}` | `virtual_router_back_refs` | compute ที่ VM อยู่ |
| `GET /virtual-router/{uuid}` | `virtual_router_ip_address` | IP ของ agent |
| `GET /bgp-routers?detail=true` | `bgp_router_parameters.router_type`, `address` | รายชื่อ control node |
| `GET /virtual-routers?detail=true` | `fq_name`, `virtual_router_ip_address` | รายชื่อ compute ที่ `plumb doctor` probe |

## Control node introspect

Stage `control` เรียก control node ทุกตัวที่ port 8083 ชื่อ field ต่างกันตาม release

| Call | Element ที่อ่าน | ใช้ทำอะไร |
| --- | --- | --- |
| `GET /Snh_ShowBgpNeighborSummaryReq` | `BgpNeighborResp`: `peer`, `peer_address`, `encoding`, `state` | session Extensible Messaging and Presence Protocol (XMPP) กับ compute |
| `GET /Snh_ShowRouteReq?routing_table={ri}.inet.0&prefix={ip}/32` | `ShowRoutePath` ใน `ShowRoute` ใน `ShowRouteTable` | route ของ VM |

Field ของ `ShowRoutePath` ที่ plumb อ่านมีดังนี้:

- `protocol`: ค่า `XMPP` แปลว่าได้ route มาจาก agent
- `source`: ชื่อ peer ที่ส่ง route มา
- `next_hop`: IP ของ compute ที่ VM อยู่
- `label`: Multiprotocol Label Switching (MPLS) label ของ interface
- `tunnel_encap`: overlay ที่ใช้ได้ เช่น `gre` หรือ `udp`
- `origin_vn`: VN ต้นทางของ route

## vRouter agent introspect

Stage `vrouter` เรียก agent บน compute ที่ port 8085 ชื่อ field ต่างกันตาม release

| Call | Element ที่อ่าน | ใช้ทำอะไร |
| --- | --- | --- |
| `GET /Snh_AgentXmppConnectionStatusReq` | `AgentXmppData`: `controller_ip`, `state`, `cfg_controller` | control node ที่ agent ต่ออยู่ |
| `GET /Snh_ItfReq?uuid={port_id}` | `ItfSandeshData`: `name`, `vrf_name`, `active`, `label` | tap interface |
| `GET /Snh_VrfListReq?name={vrf}` | `VrfSandeshData`: `ucindex` | index ที่ใช้ค้น route |
| `GET /Snh_Inet4UcRouteReq?vrf_index={n}&src_ip={ip}&prefix_len=32` | `NhSandeshData` ใน `PathSandeshData` ใน `RouteUcSandeshData` | next hop ของ VM |
| `GET /Snh_FetchAllFlowRecords` | `SandeshFlowData`: `sip`, `dip`, `src_port`, `dst_port`, `protocol`, `drop_reason` | flow ของ VM |

`FetchAllFlowRecords` ตอบเฉพาะหน้าแรกของ flow table จำนวน flow ใน tree จึงเป็นตัวอย่าง ไม่ใช่จำนวนทั้งหมด

Field `type` ของ `NhSandeshData` บอกชนิดของ next hop:

- `interface`: VM อยู่บน compute นี้
- `tunnel`: ส่งต่อไป compute อื่น โดย `dip` เป็น IP ปลายทาง และ `tunnel_type` เป็นชนิดของ overlay
