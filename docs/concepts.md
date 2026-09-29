<!-- contentType: Conceptual · plan: docs/content-plan.md -->

# request ของ VM ผ่านชั้นไหนบ้าง และแต่ละชั้นเชื่อมกันด้วยอะไร

หน้านี้อธิบายว่า VM หนึ่งเครื่องบน OpenStack ที่ใช้ OpenSDN ต้องผ่าน component ใดบ้างก่อนส่ง packet ได้ และแต่ละ component ส่งข้อมูลต่อกันด้วยค่าอะไร ทุกหัวข้อชี้ไปที่บรรทัดใน tree ของ plumb ที่ใช้ยืนยันเรื่องนั้น

หน้านี้เรียกลำดับ object ที่เชื่อมจาก VM ถึง route บน vRouter agent ว่า chain และใช้ชื่อ object ของ OpenSDN 4 ตัว virtual machine interface หรือ VMI คือ port ฝั่ง OpenSDN virtual network หรือ VN คือ network, routing instance หรือ RI คือ routing table ของ VN และ route target หรือ RT คือป้ายที่บอกว่า route ไหน import เข้า RI ไหนได้

## ภาพรวมของ chain

ตอนสร้าง VM ข้อมูลไหลจาก API ของ OpenStack ลงไปถึง kernel module บน compute ตามลำดับนี้:

```text
openstack server create
  → Nova         เลือก compute และขอ port จาก Neutron
  → Neutron      ส่งต่อให้ OpenSDN plugin
  → Config API   เก็บ VMI และ VN
  → Schema transformer  สร้าง RI และ RT
  → Control node ส่ง config ให้ agent ผ่าน XMPP
  → vRouter agent  สร้าง tap, VRF และ route
  → Control node รับ route ของ VM แล้วกระจายต่อ
```

plumb อ่าน chain ในทิศเดียวกัน แต่ละชั้นใช้ค่าจากชั้นก่อนหน้าเป็น key:

| จากชั้น | ค่าที่ใช้ต่อ | ไปถึงชั้น |
| --- | --- | --- |
| Keystone | service catalog | endpoint ของ Nova และ Neutron |
| Nova | UUID ของ VM | port ที่มี `device_id` เท่ากัน |
| Neutron | UUID ของ port | VMI ที่มี UUID เดียวกัน |
| Config | `routing_instance_refs` ของ VMI | ชื่อ route table บน control node |
| Config | `virtual_router_back_refs` ของ VM | IP ของ vRouter agent |
| Agent | UUID ของ port | tap interface และ VRF |

## Neutron ไม่ได้ต่อ network เอง

Neutron รับ API ของ network, subnet และ port แล้วส่งต่อให้ core plugin ของ backend บน OpenSDN plugin แปลงทุก call เป็น object ใน Config API และใช้ UUID เดิม port หนึ่งตัวจึงเป็น VMI ที่มี UUID เดียวกัน

ค่า `binding:vif_type` ของ port บอกว่า backend ตัวไหนเสียบ port เข้า datapath บน OpenSDN ค่านี้คือ `vrouter` ตอนสร้าง VM Nova อ่านค่านี้แล้วเสียบ tap interface เข้า vRouter แทน Open vSwitch

tree ของ plumb แสดงเรื่องนี้ใน 2 บรรทัด:

- `Neutron  ACTIVE  vif_type=vrouter`
- `Config  VMI …  ✓ same UUID as the port`

## Config แยกจาก control node เพราะ intent ต่างจาก routing

Config API เก็บสิ่งที่ผู้ใช้ต้องการ เช่น VN ชื่อ `vn1` ที่ต่อกับ policy หนึ่งตัว แต่ control node ต้องใช้ข้อมูล routing เช่น RI และ RT ซึ่งผู้ใช้ไม่ได้สร้างเอง

Schema transformer เป็น process ที่อ่าน intent จาก Config API แล้วสร้าง object ของ routing ให้:

- Schema transformer สร้าง RI 1 ตัวต่อ VN `fq_name` ของ RI คือ `fq_name` ของ VN ต่อท้ายด้วยชื่อ VN อีกครั้ง เช่น `default-domain:admin:vn1:vn1`
- Schema transformer สร้าง RT ให้ RI อัตโนมัติ เลขอยู่ในช่วงที่ schema transformer จองไว้ เช่น `target:64512:8000002` ใน lab จำลอง
- เมื่อ policy อนุญาตให้ 2 VN คุยกัน schema transformer ให้ RI ของแต่ละฝั่ง import RT ของอีกฝั่ง

การแยก 2 ชั้นทำให้ผู้ใช้เปลี่ยน intent ได้โดยไม่ต้องรู้เรื่อง routing ผมเห็นว่าข้อดีนี้คุ้ม แต่มีราคา schema transformer เป็น process อีกตัวที่หยุดทำงานได้ขณะที่ Config API ยังตอบปกติ ถ้า VN มีแต่ไม่มี RI แปลว่า schema transformer ยังไม่ได้ประมวลผล VN นั้น plumb จึงตรวจ RI แยกจาก VN และเตือนใน stage `opensdn-config`

## XMPP ส่งข้อมูลระหว่าง control node กับ agent

Extensible Messaging and Presence Protocol (XMPP) เป็นช่องทางระหว่าง control node กับ vRouter agent ข้อมูลไหลทั้ง 2 ทิศ:

- Control node ส่ง config ของ VMI, VN และ RI ที่ agent ต้องใช้ ให้ agent
- Agent ส่ง route ของ VM ที่อยู่บน compute นั้นให้ control node พร้อม next hop เป็น IP ของ compute และ label ของ interface

Agent ต่อกับ control node ได้สูงสุด 2 ตัว ถ้าตัวหนึ่งล่ม agent ยังรับและส่ง route ผ่านอีกตัวได้ Agent เลือก control node 1 ตัวเป็นแหล่ง config และ tree แสดงคำว่า `config` ท้ายบรรทัด `XMPP` ของ control node ตัวนั้น

บรรทัด `path  XMPP from compute-02  nh 10.10.0.21  label 25` ใน tree แปลว่า control node ได้ route ของ VM จาก agent บน `compute-02` แล้ว ถ้า route หายไป ปัญหาอยู่ระหว่าง agent กับ control node ไม่ใช่ที่ Config API

## BGP ส่ง route ระหว่าง control node และไปยัง gateway

Control node คุยกันเองและคุยกับ gateway router ด้วย Border Gateway Protocol (BGP) แบบ layer 3 VPN (L3VPN) ซึ่งเป็นแบบเดียวกับที่ผู้ให้บริการ network ใช้แยก VPN ของลูกค้า ผลที่ได้มี 2 ข้อ:

- Control node ทุกตัวเห็น route ชุดเดียวกัน แม้ agent จะต่อกับ control node คนละตัว
- Gateway router รับ route ของ VM ไปใช้ได้ เช่นตอน VM ใช้ floating IP

plumb อ่านรายการ peer จาก `Snh_ShowBgpNeighborSummaryReq` ซึ่งรวมทั้ง peer ของ BGP และ XMPP ไว้ด้วยกัน แล้วแยกด้วย field `encoding`

## VRF แยก tenant ออกจากกัน

Virtual routing and forwarding (VRF) คือ routing table แยกต่อ RI บน agent ทุก VM ใน VN เดียวกันอยู่ใน VRF เดียวกัน และ VRF หนึ่งมีเฉพาะ route ที่มี RT ตรงกับ RT ที่ RI ของ VRF นั้น import

Tenant 2 รายที่ใช้ subnet `10.0.1.0/24` ซ้ำกันจึงไม่ชนกัน เพราะ route ของแต่ละรายอยู่คนละ VRF และมี RT คนละตัว

ใน tree ชื่อ RI ในบรรทัด `Config`, ชื่อ table ในบรรทัด `Control` และชื่อ `vrf` ในบรรทัด `vRouter` มาจาก `default-domain:admin:vn1:vn1` ชื่อเดียวกัน

## Overlay อยู่ที่ encap และ label

vRouter ห่อ packet ระหว่าง VM ที่อยู่คนละ compute ด้วย header ของ overlay แล้วส่งข้าม underlay ไปที่ IP ของ compute ปลายทาง OpenSDN รองรับ 3 แบบ:

- MPLSoUDP ใส่ label ของ Multiprotocol Label Switching หรือ MPLS ไว้ใน UDP และแสดงใน tree เป็น `udp`
- MPLSoGRE ใส่ MPLS label ไว้ใน Generic Routing Encapsulation หรือ GRE และแสดงใน tree เป็น `gre`
- VXLAN หรือ Virtual Extensible LAN ใช้ VXLAN network identifier ของ VN แทน label ใช้กับ route ของ layer 2 แบบ EVPN และแสดงใน tree เป็น `vxlan`

Label บอก compute ปลายทางว่าต้องส่ง packet เข้า interface ไหน label จึงต้องตรงกันทุกชั้น plumb เทียบ label ที่ control node ประกาศกับ label ที่ agent กำหนดให้ tap interface และเตือนเมื่อไม่ตรงกัน

Route ของ VM บน compute เดียวกันแสดง next hop เป็น `local interface` ส่วน route ของ VM บน compute อื่นแสดงเป็น `tunnel MPLSoUDP to` ตามด้วย IP ของ compute นั้น
