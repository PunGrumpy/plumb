<!-- contentType: Troubleshooting · plan: docs/content-plan.md -->

# คำเตือนแต่ละข้อของ plumb หมายถึงอะไร

หน้านี้อธิบาย code ทุกตัวที่ plumb แสดงในบรรทัด `More  plumb explain <code>` และในฟิลด์ `code` ของ JSON แต่ละหัวข้อชื่อตาม code บอกความหมายและสิ่งที่ต้องตรวจต่อ คำเตือนแปลว่า API ตอบปกติแต่ข้อมูลระหว่างชั้นไม่ตรงกัน ส่วน error แปลว่าเรียก API ไม่สำเร็จ

คำสั่ง `plumb explain <code>` แสดงเนื้อหาเดียวกันเป็นภาษาอังกฤษโดยไม่ต้องเปิดหน้านี้

## Credential และการเชื่อมต่อ

Code กลุ่มนี้เกิดได้ในทุก stage และทุก check ของ `plumb doctor`

### `no-credentials`

Shell ยังไม่มี variable `OS_*` ให้ source openrc แล้วรันใหม่ ถ้ายังไม่มี openrc ให้ลอง `plumb demo` ซึ่งไม่ต้องใช้ credential

### `auth-failed`

Keystone ตอบ HTTP 401 username, password หรือ project ไม่ถูกต้อง ให้ลอง `openstack token issue` ด้วย environment เดียวกัน

### `forbidden`

Token ใช้ได้ แต่ role ของ token อ่าน object นั้นไม่ได้ ให้ใช้ user ที่มี role `admin` หรือ `reader` ใน project

### `unreachable`

เครื่องที่รัน plumb เปิดการเชื่อมต่อไปที่ endpoint ไม่ได้ ข้อความ error บอกสาเหตุ เช่น `connection refused` หรือ `no such host` ให้รัน `plumb doctor` บนเครื่องเดียวกันเพื่อดูว่าเข้าถึง endpoint ไหนได้บ้าง ถ้า IP ที่ config บันทึกไว้เข้าไม่ถึง ให้ใช้ `--control-url` หรือ `--agent-url`

### `timeout`

Endpoint รับการเชื่อมต่อแต่ไม่ตอบภายในเวลาที่กำหนด ให้เพิ่ม `--request-timeout` หรือตรวจ load ของ service นั้น

### `no-recording`

Directory ที่ส่งให้ `--replay` ไม่มีไฟล์ของ call นี้ ให้บันทึก lab ใหม่ด้วย `--record` โดยใช้ flag ชุดเดียวกัน

## Stage `keystone`

ถ้า stage นี้ fail plumb ข้าม stage `nova` และ `neutron`

### `endpoint-missing`

Service catalog ใน token ไม่มี endpoint ของ service ที่ plumb ต้องใช้ ให้เทียบ `OS_REGION_NAME` และ `OS_INTERFACE` กับผลของ `openstack catalog list`

### `no-admin`

Token ไม่มี role `admin` Nova จึงไม่แสดง host ของ VM ถ้า cloud ใช้ OpenSDN plumb ยังหา compute ได้จาก `virtual-router` ใน Config API

## Stage `nova` และ `neutron`

Code ใน 2 stage นี้เกี่ยวกับ project ที่ token ใช้ หรือ host ที่ port ผูกอยู่

### `vm-not-found`

ไม่มี server ที่มี UUID หรือชื่อนี้ในส่วนที่ token มองเห็น token ที่ไม่ใช่ admin เห็นเฉพาะ project ของตัวเอง ส่วน token ที่มี role `admin` ทำให้ plumb ค้นชื่อในทุก project ให้ตรวจชื่อด้วย `openstack server list --all-projects` หรือ source openrc ของ project ที่เป็นเจ้าของ VM

### `vm-ambiguous`

มี server มากกว่า 1 เครื่องที่ใช้ชื่อหรือ IP นี้ IP ซ้ำกันได้เมื่อ network ของต่าง project ใช้ subnet เดียวกัน ข้อความ error แสดง UUID ของทุกเครื่อง ให้รันใหม่ด้วย UUID ที่ต้องการ

### `ip-not-found`

ไม่มี port ใดใน Neutron ที่ใช้ IP นี้เป็น fixed IP และไม่มี floating IP ที่มี address นี้ ให้ตรวจด้วย `openstack port list --fixed-ip ip-address=your_ip` และ `openstack floating ip list` token ที่ไม่ใช่ admin เห็นเฉพาะ port ของ project ตัวเอง

### `ip-not-vm`

IP มีอยู่จริง แต่อยู่บน port ที่ไม่ใช่ของ VM เช่น router interface, DHCP port หรือ floating IP ที่ยังไม่ได้ผูกกับ port ใด ข้อความ error บอก `device_owner` ของ port นั้น ให้ trace VM ที่อยู่หลังอุปกรณ์นั้นแทน

### `server-not-active`

Nova รายงานสถานะของ server ที่ไม่ใช่ `ACTIVE` ให้รัน `openstack server show your_vm_name` แล้วอ่านฟิลด์ `fault`

### `lookup-failed`

plumb อ่าน object ที่เกี่ยวข้องไม่ได้ 1 ตัว ส่วนอื่นของ trace ยังถูกต้อง ให้รันใหม่ด้วย `--debug` เพื่อดู call ที่ fail และ status code

### `no-ports`

Neutron ไม่มี port ที่ `device_id` เป็น VM นี้ ให้ตรวจด้วย `openstack port list --server your_vm_name` VM ที่ไม่มี port ไม่มี network

### `binding-failed`

Neutron ไม่มี mechanism driver ตัวใดที่เสียบ port เข้า datapath บน host นั้นได้ ให้อ่าน log ของ `neutron-server` บน controller และตรวจว่า agent ของ network บน compute ยังทำงานอยู่

### `port-not-active`

Port ใน Neutron ยังไม่ `ACTIVE` แปลว่า backend ยังเสียบ port ไม่เสร็จ ให้ตรวจ agent ของ network บน compute ที่ port ผูกอยู่

### `host-mismatch`

Nova, Neutron และ OpenSDN เห็น compute ของ VM ไม่ตรงกัน ให้ตรวจ migration ที่ค้างอยู่ด้วย `openstack server migration list`

## Stage `opensdn-config`

Code ใน stage นี้บอกว่า Neutron กับ OpenSDN sync กันหรือไม่ และ schema transformer ทำงานแล้วหรือยัง

### `vmi-missing`

Port มีใน Neutron แต่ไม่มี virtual machine interface (VMI) ที่ UUID เดียวกันใน OpenSDN สาเหตุมี 2 แบบ:

- Cloud นี้ไม่ได้ใช้ OpenSDN plugin ถ้า `vif_type` ของ port ไม่ใช่ `vrouter` ให้รันโดยไม่มี `--config-url`
- 2 ระบบไม่ sync กัน ให้อ่าน log ของ OpenSDN plugin ใน `neutron-server`

### `no-routing-instance`

Virtual network (VN) มีแล้วแต่ยังไม่มี routing instance (RI) แปลว่า schema transformer ยังไม่ได้ประมวลผล VN นี้ ให้ตรวจว่า process `contrail-schema` ทำงานอยู่และไม่มี error ใน log

### `no-route-target`

RI ไม่มี route target (RT) VRF อื่นจึง import route ของ RI นี้ไม่ได้ ให้ตรวจ `contrail-schema` แบบเดียวกับ `no-routing-instance`

### `compute-unknown`

plumb ไล่จาก `virtual-machine` ไป `virtual-router` ไม่สำเร็จ จึงไม่รู้ IP ของ vRouter agent ให้ใช้ `--agent-url` ชี้ไปที่ introspect ของ compute ที่ VM อยู่

### `control-discovery`

plumb อ่านรายการ `bgp-router` เพื่อหา control node ไม่ได้ ให้ใช้ `--control-url`

## Stage `control`

Code ใน stage นี้บอกว่า agent ประกาศ route ของ VM ให้ control node แล้วหรือยัง

### `control-unreachable`

plumb อ่าน introspect ของ control node ที่ port 8083 ไม่ได้ ให้รัน `plumb doctor` หรือใช้ `--control-url` ชี้ไปที่ address ที่เครื่องนี้เข้าถึงได้

### `xmpp-missing`

Control node ไม่มี session Extensible Messaging and Presence Protocol (XMPP) ที่อยู่ในสถานะ `Established` กับ agent บน compute ของ VM ให้ดูบรรทัด `XMPP` ใต้ `Compute` ซึ่งแสดง session จากฝั่ง agent

### `route-missing`

Session อาจปกติ แต่ route ของ VM ไม่อยู่ใน table ของ RI บน control node ให้ตรวจ 3 ข้อนี้ตามลำดับ:

1. บรรทัด `vRouter` ของ port ต้องแสดง `✓ active`
2. Agent อาจต่อกับ control node ตัวอื่นเท่านั้น ให้ตรวจว่า session BGP ระหว่าง control node อยู่ในสถานะ `Established`
3. ชื่อ RI ใน Config API ต้องตรงกับชื่อ VRF บน agent

## Stage `vrouter`

Code ใน stage นี้บอกสถานะบน compute ที่ VM อยู่ และตรวจว่า label ตรงกับที่ control node ประกาศ

### `agent-xmpp-down`

Agent ไม่มี session XMPP ที่ `Established` กับ control node ตัวใด control node จึงไม่ได้รับ route จาก compute นี้ ให้ตรวจ network จาก compute ไปที่ control node ที่ TCP port 5269

### `interface-missing`

Agent ไม่รู้จัก port นี้ Nova อาจยังไม่ได้เสียบ tap หรือ agent ยังไม่ได้รับ config ของ VMI ให้อ่าน log ของ `nova-compute`

### `interface-inactive`

Agent รู้จัก interface แต่ยังไม่เปิดใช้ ให้ตรวจว่า agent ได้รับ config ของ VN และ IP ของ VM แล้ว

### `label-mismatch`

Label ที่ control node ประกาศไม่ตรงกับ label ที่ agent ใช้อยู่ compute อื่นจึงส่ง packet ด้วย label ที่ agent ไม่รู้จัก สาเหตุหนึ่งคือ control node ยังเก็บ route เก่าไว้ ให้เปิด `Snh_ShowRouteReq` ของ prefix นั้นใน control introspect แล้วดูเวลาที่ route เปลี่ยนครั้งล่าสุด

### `agent-route-missing`

VRF บน agent ไม่มี route ของ IP ของ VM ให้ตรวจว่า IP ใน Neutron ตรงกับ IP ที่ VM ใช้อยู่

## คำสั่ง `plumb path`

Code กลุ่มนี้มาจากการตรวจเส้นทางระหว่าง VM 2 เครื่อง บรรทัด `Hint` ของ `plumb path` บอกคำสั่งที่แก้ปัญหาของ path นั้นโดยตรง

### `path-no-router`

VM ทั้งสองอยู่คนละ subnet และไม่มี router ของ Neutron ตัวใดที่มี interface บนทั้ง 2 network ให้ต่อ subnet ทั้งสองเข้ากับ router ตัวเดียวกันด้วย `openstack router add subnet` หรือเรียก VM ปลายทางผ่าน floating IP

### `sg-egress-blocked`

ไม่มี egress rule ใน security group ของ VM ต้นทางที่ปล่อย traffic นี้ไปยังปลายทาง security group เริ่มต้นของ OpenStack ปล่อย egress ทั้งหมด code นี้จึงแปลว่ามีคนลบ rule นั้นออก หรือ port ใช้ security group อื่น

### `sg-ingress-blocked`

ไม่มี ingress rule ใน security group ของ VM ปลายทางที่ปล่อย traffic นี้จาก VM ต้นทาง security group `default` ปล่อย ingress เฉพาะจาก port ที่อยู่ใน group เดียวกัน VM ต้นทางที่อยู่คนละ group จึงถูกกันไว้ ให้เพิ่ม rule ตามคำสั่งในบรรทัด `Hint` หรือใช้ `--remote-group` แทน `--remote-ip` ถ้าต้องการปล่อยทั้ง group

### `path-no-route`

VRF ของ VM ต้นทางบน compute ของมันไม่มี route ไป IP ปลายทาง ถ้า VM อยู่คนละ VN routing instance ของต้นทางต้อง import route target ของปลายทาง ให้ตรวจ network policy หรือ logical router ระหว่าง 2 VN ถ้าอยู่ VN เดียวกัน ให้รัน `plumb trace` กับปลายทางเพื่อดูว่า control node มี route ของมันหรือไม่

### `path-wrong-next-hop`

VRF ต้นทางมี route แต่ route นั้นไม่ได้ไปที่ compute หรือ interface ที่ VM ปลายทางอยู่ สาเหตุหนึ่งคือ control node ยังเก็บ route เก่าหลัง VM ย้าย compute ให้รัน `plumb trace` กับปลายทางแล้วเทียบบรรทัด `Control` กับ compute ปัจจุบัน
