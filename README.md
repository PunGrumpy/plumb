<!-- contentType: Landing · plan: docs/content-plan.md -->

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/wordmark-white.svg">
    <img src="brand/wordmark-black.svg" alt="plumb" width="240">
  </picture>
</p>

# plumb ไล่ VM หนึ่งเครื่องผ่านทุกชั้นของ OpenStack และ OpenSDN

plumb เป็น CLI ภาษา Go รับชื่อหรือ UUID ของ VM แล้วไล่ข้อมูลจาก Keystone, Nova และ Neutron ลงไปถึง OpenSDN Config API, control node และ vRouter agent บน compute ที่ VM อยู่ ผลลัพธ์เป็น tree เดียวที่เห็นทุกชั้น

ลำดับ object ที่เชื่อมจาก VM ถึง route บน vRouter agent เรียกว่า chain ถ้าข้อมูลของ 2 ชั้นไม่ตรงกัน plumb บอกว่า chain หยุดที่ชั้นไหน ต้องตรวจอะไรต่อ และคำสั่งที่อธิบายปัญหานั้น

## สิ่งที่ plumb แสดง

plumb เรียกเฉพาะ API ที่อ่านข้อมูล และแสดงผลของ 6 ชั้นในตารางนี้ ชั้นที่ 5 และ 6 เป็น introspect ซึ่งคือหน้า HTTP สำหรับ debug ที่ process ของ OpenSDN เปิดไว้

| ชั้น | Port | สิ่งที่ plumb แสดง |
| --- | --- | --- |
| Keystone | ตาม `OS_AUTH_URL` | token, role และ endpoint จาก service catalog |
| Nova | ตาม catalog | host, flavor, image และ port ที่ attach อยู่ |
| Neutron | ตาม catalog | port, network, subnet, security group และ floating IP |
| OpenSDN Config API | 8082 | virtual machine interface หรือ VMI, virtual network หรือ VN, routing instance หรือ RI และ route target หรือ RT |
| Control node introspect | 8083 | route ของ VM และ session XMPP กับ compute |
| vRouter agent introspect | 8085 | tap interface, VRF, route และ flow |

XMPP ย่อมาจาก Extensible Messaging and Presence Protocol และ VRF ย่อมาจาก virtual routing and forwarding

ตัวอย่างนี้เป็นส่วน port ของผลลัพธ์จาก lab จำลอง ผมตัดบรรทัดยาวด้วย `…` คำสั่ง `plumb demo` สร้างผลลัพธ์ทั้งหมด

```text
└─ Port      9c1e4d2b-7a3f-…  fa:16:3e:5a:12:7c  10.0.1.5
   ├─ Neutron   ACTIVE  vif_type=vrouter  host=compute-02
   ├─ Config    VMI default-domain:admin:9c1e4d2b-…  ✓ same UUID as the port
   │  ├─ RI        default-domain:admin:vn1:vn1
   │  └─ RT        target:64512:8000002  import+export
   ├─ Control   control-01  vn1:vn1.inet.0  10.0.1.5/32 ✓
   │  └─ path      XMPP from compute-02  nh 10.10.0.21  label 25  encap gre,udp
   └─ vRouter   tap9c1e4d2b-7a ✓ active  vrf vn1:vn1 (index 3)  label 25
      └─ route     10.0.1.5/32  local interface tap9c1e4d2b-7a  label 25
```

## เริ่มต้นใช้งาน

Build ด้วย Go 1.24 ขึ้นไป แล้วรัน lab ที่ฝังอยู่ใน binary ขั้นนี้ไม่ต้องมี cloud หรือ credential

```sh
make build
./bin/plumb demo
```

บน cloud จริง ให้ใช้คำสั่งตามลำดับนี้

| คำสั่ง | ใช้เมื่อ |
| --- | --- |
| `plumb demo [scenario]` | ลองใช้ครั้งแรก หรือดูว่าชั้นที่พังหน้าตาเป็นอย่างไร |
| `plumb link --config-url <url>` | ครั้งแรกบนแต่ละ cloud ที่ใช้ OpenSDN เพื่อจำ URL ของ Config API |
| `plumb doctor` | ครั้งแรกบนแต่ละเครื่อง เพื่อดูว่าเครื่องนั้นเข้าถึงชั้นไหนได้ |
| `plumb <vm>` | ไล่ VM จากชื่อ, UUID, fixed IP หรือ floating IP |
| `plumb path <from> <to>` | ได้รับแจ้งว่า VM สองเครื่องคุยกันไม่ได้ |
| `plumb explain <code>` | อ่านความหมายของ code ในบรรทัด `More` |
| `plumb whoami` | ดู user, project และ URL ของ OpenSDN ที่ plumb จะใช้ |

## เอกสาร

เลือกหน้าตามงานที่คุณจะทำ

| หน้า | อ่านเมื่อ |
| --- | --- |
| [รัน plumb ครั้งแรกกับ lab จำลอง](docs/quickstart.md) | อยากเห็นผลลัพธ์ทีละชั้นก่อนใช้กับ cloud จริง |
| [วิธีใช้ plumb กับ DevStack และ lab OpenSDN](docs/run-against-a-lab.md) | จะติดตั้งบน server, link cloud และบันทึก lab |
| [ตัวเลือกของ plumb](docs/cli-reference.md) | ต้องการรายการคำสั่ง, flag, environment variable, exit code และ JSON |
| [request ของ VM ผ่านชั้นไหนบ้าง](docs/concepts.md) | อยากเข้าใจ Neutron plugin, schema transformer, XMPP, BGP, VRF และ overlay |
| [คำเตือนแต่ละข้อของ plumb หมายถึงอะไร](docs/troubleshooting.md) | เจอ code ที่ไม่รู้จัก |
| [API ที่ plumb เรียกในแต่ละชั้น](docs/api-reference.md) | ต้องเทียบ endpoint และ field กับ lab |
| [plumb ออกแบบอย่างไร](docs/architecture.md) | จะแก้โค้ด |
| [วิธีเพิ่ม stage ใหม่ให้ plumb](docs/add-a-stage.md) | จะเพิ่ม API ใหม่เข้า trace |

## สิ่งที่ต้องขอก่อนใช้กับ environment จริง

Introspect ของ OpenSDN ไม่มี authentication ก่อนใช้กับ environment จริง ให้ขอสิทธิ์เข้า port 8082, 8083 และ 8085 จากพี่หมู plumb ไม่ส่ง Keystone token ไปที่ introspect และไม่เรียก API ที่แก้ไขข้อมูล

โลโก้และกฎการใช้งานอยู่ใน [brand asset ของ plumb](brand/README.md)
