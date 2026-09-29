<!-- contentType: Reference · plan: docs/content-plan.md -->

# ตัวเลือกของ plumb

หน้านี้รวมคำสั่ง, flag, environment variable, exit code และรูปแบบ JSON ของ `plumb` ตรงกับโค้ดใน `cmd/plumb` และผลของ `plumb <command> -h`

## คำสั่ง

`plumb` มี 9 คำสั่ง ถ้าไม่ใส่คำสั่ง plumb จะพิมพ์ help

| คำสั่ง | หน้าที่ |
| --- | --- |
| `plumb trace <vm>` | ไล่ VM จาก UUID, ชื่อ, fixed IP หรือ floating IP |
| `plumb <vm>` | ทางลัดของ `plumb trace <vm>` |
| `plumb path <from> <to>` | ตรวจว่า VM หนึ่งส่ง traffic ถึงอีกเครื่องได้หรือไม่ |
| `plumb link` | จำ URL ของ OpenSDN สำหรับ cloud ใน `OS_AUTH_URL` แสดง link ปัจจุบันเมื่อไม่มี flag |
| `plumb whoami` | แสดง user, project, role และ URL ของ Config API พร้อมที่มาของ URL |
| `plumb demo [scenario]` | ไล่ `web-01` ใน lab ที่ฝังใน binary ไม่ต่อ network |
| `plumb doctor` | ตรวจ credential และ endpoint ทุกตัวที่ trace ต้องใช้ |
| `plumb explain [code]` | อธิบาย code ของคำเตือน ถ้าไม่ใส่ code จะแสดงทุก code |
| `plumb version` | พิมพ์ version |

ถ้าพิมพ์ชื่อคำสั่งผิด plumb จะเสนอชื่อที่ใกล้ที่สุด `<vm>` เป็น UUID, ชื่อ หรือ IP ก็ได้ ถ้าเป็น IP stage `nova` ค้น port ที่มี fixed IP นั้นใน Neutron ถ้าไม่เจอจะค้น floating IP แล้วใช้ `device_id` ของ port เป็น VM `<vm>` ที่ไม่ใช่ UUID และไม่ใช่ IP ถือเป็นชื่อ stage `nova` ค้น server ที่ชื่อตรงกันทุกตัวอักษรใน project ของ token ถ้าไม่เจอและ token มี role `admin` จะค้นต่อในทุก project ใส่ flag ได้ทั้งก่อนและหลัง `<vm>`

## Scenario ของ demo

`plumb demo --list` แสดงรายการนี้:

| Scenario | ชั้นที่พัง | Exit code |
| --- | --- | --- |
| `healthy` | ไม่มี เป็นค่าเริ่มต้น | `0` |
| `devstack` | ไม่มี Config API จึงข้าม 3 stage ของ OpenSDN | `0` |
| `vmi-missing` | port ใน Neutron ไม่มี VMI ใน OpenSDN | `1` |
| `missing-route` | `control-02` ไม่มี route ของ VM | `0` |
| `label-mismatch` | agent ใช้ label ที่ control node ไม่ได้ประกาศ | `0` |
| `agent-down` | introspect ของ agent ปฏิเสธการเชื่อมต่อ | `1` |

## Environment variable ของ OpenStack

`plumb <vm>` และ `plumb doctor` อ่าน variable ชุดเดียวกับที่ openrc export:

| Variable | ต้องมี | ค่าเริ่มต้นและหมายเหตุ |
| --- | --- | --- |
| `OS_AUTH_URL` | ใช่ | เติม `/v3` ท้าย URL ถ้าไม่มี |
| `OS_USERNAME`, `OS_PASSWORD` | ใช่ ยกเว้นใช้ application credential | |
| `OS_PROJECT_NAME` หรือ `OS_PROJECT_ID` | ใช่ ยกเว้นใช้ application credential | อ่าน `OS_TENANT_NAME` และ `OS_TENANT_ID` แทนได้ |
| `OS_USER_DOMAIN_NAME` หรือ `OS_USER_DOMAIN_ID` | ไม่ | `Default` |
| `OS_PROJECT_DOMAIN_NAME` หรือ `OS_PROJECT_DOMAIN_ID` | ไม่ | `Default` |
| `OS_APPLICATION_CREDENTIAL_ID`, `OS_APPLICATION_CREDENTIAL_SECRET` | ไม่ | ใช้แทน username และ password |
| `OS_REGION_NAME` | ไม่ | ว่าง แปลว่ารับ endpoint ของ region แรกใน catalog |
| `OS_INTERFACE` | ไม่ | `public` และรับ `publicURL` ได้ |

## Check ของ `plumb path`

`plumb path` trace VM ทั้ง 2 เครื่องแล้วตรวจตามลำดับที่ packet ผ่าน

| Check | ผ่านเมื่อ | ต้องใช้ |
| --- | --- | --- |
| `resolve` | VM ทั้ง 2 มี port ใน Neutron และมี IPv4 | ทุก cloud |
| `ports` | port ทั้ง 2 เป็น `ACTIVE` | ทุก cloud |
| `network` | IP ปลายทางอยู่ใน subnet ของต้นทาง หรือมี router ตัวเดียวกันต่อทั้ง 2 network | ทุก cloud |
| `egress` | security group ของต้นทางมี egress rule ที่ปล่อย traffic ไป IP ปลายทาง หรือปิด port security | ทุก cloud |
| `ingress` | security group ของปลายทางมี ingress rule ที่ปล่อย traffic จาก IP หรือ group ของต้นทาง หรือปิด port security | ทุก cloud |
| `route` | VRF ของต้นทางบน compute ต้นทางมี route ไป IP ปลายทาง | OpenSDN |
| `next-hop` | route ส่งเข้า tap ของปลายทางบน compute เดียวกัน หรือ tunnel ไป compute ของปลายทางด้วย label เดียวกับ interface ของปลายทาง | OpenSDN |

Security group เป็นแบบ stateful plumb จึงตรวจเฉพาะ egress ของต้นทางและ ingress ของปลายทาง ICMP หมายถึง echo request แบบที่ `ping` ส่ง ส่วน `plumb path` ยังไม่ตรวจ IPv6, floating IP ที่เรียกจากนอก cloud และ network policy ของ OpenSDN โดยตรง

| Flag | ค่าเริ่มต้น | หน้าที่ |
| --- | --- | --- |
| `--proto` | `icmp` หรือ `tcp` ถ้าใส่เฉพาะ `--port` | protocol ที่ตรวจ `icmp`, `tcp` หรือ `udp` |
| `--port` | ไม่มี | port ปลายทาง ต้องใส่เมื่อใช้ `tcp` หรือ `udp` |

`plumb path` รับ flag ของ endpoint, output, `--record` และ `--replay` เหมือน `plumb trace` และจบด้วย exit code `1` เมื่อ check ใดไม่ผ่าน

## Flag ของ `plumb link`

| Flag | หน้าที่ |
| --- | --- |
| `--config-url` | URL ของ Config API ที่จะจำ |
| `--control-url` | URL ของ control introspect ที่จะจำ คั่นด้วย comma |
| `--remove` | ลบ link ของ cloud นี้ |
| `--force` | บันทึกแม้ Config API ไม่ตอบจากเครื่องนี้ |

ก่อนบันทึก `plumb link` ส่ง `GET` ไปที่ `--config-url` ถ้าไม่มี HTTP response กลับมา คำสั่งจะจบด้วย exit code `1` และไม่บันทึก

## ที่มาของ URL ของ OpenSDN

`trace`, `doctor` และ `whoami` เลือก URL ของ Config API และ control node จากที่แรกที่มีค่าในลำดับนี้

1. Flag `--config-url` และ `--control-url`
2. Variable `OPENSDN_CONFIG_URL` และ `OPENSDN_CONTROL_URLS`
3. Link ของ cloud ที่ตรงกับ `OS_AUTH_URL` ใน config file

Config file อยู่ที่ `$XDG_CONFIG_HOME/plumb/config.json` หรือ `~/.config/plumb/config.json` ถ้าไม่ได้ตั้ง `XDG_CONFIG_HOME` variable `PLUMB_CONFIG` เปลี่ยน path ของไฟล์ได้ plumb ถือว่า `OS_AUTH_URL` ที่ต่างกันเพียง `/v3` หรือ `/` ท้าย URL เป็น cloud เดียวกัน และเขียนไฟล์ด้วย permission `0600` เพราะไฟล์มี address ภายใน

## การแจ้งเตือน version ใหม่

หลังคำสั่งจบ plumb บอกบน stderr เมื่อมี release ที่ใหม่กว่า version ที่ใช้อยู่ plumb อ่าน release ล่าสุดจาก URL ที่ตั้งตอน build และเก็บผลไว้ใน `$XDG_CACHE_HOME/plumb/update.json` หรือ `~/.cache/plumb/update.json` นาน 24 ชั่วโมง การเช็กทำงานพร้อมกับคำสั่ง และรอหลังคำสั่งจบไม่เกิน 500 ms

plumb ไม่เช็กในกรณีเหล่านี้

- stderr ไม่ใช่ terminal
- ตั้ง `CI` หรือ `PLUMB_NO_UPDATE_CHECK`
- ใช้ `--json`
- binary เป็น version `dev`
- build โดยไม่ตั้ง `UPDATE_URL`

URL ต้องตอบเป็น JSON แบบ GitHub releases API ที่มี `tag_name` และ `html_url` หรือแบบที่มี `version` และ `url` variable `PLUMB_UPDATE_URL` ใช้แทน URL ที่ตั้งตอน build ได้

## Environment variable ของ terminal

| Variable | ผล |
| --- | --- |
| `NO_COLOR` | ปิดสี |
| `FORCE_COLOR` | เปิดสีแม้ stdout ไม่ใช่ terminal ยกเว้นค่า `0` |
| `TERM=dumb` | ปิดสี |
| `CI` | ปิด spinner |

plumb แสดงสีเมื่อ stdout เป็น terminal และแสดง spinner บน stderr เมื่อ stderr เป็น terminal

## Flag ของ `plumb <vm>` และ `plumb doctor`

แต่ละ flag ของ OpenSDN อ่านค่าเริ่มต้นจาก environment variable ในคอลัมน์ที่ 3:

| Flag | ค่าเริ่มต้น | Variable | หน้าที่ |
| --- | --- | --- | --- |
| `--config-url` | ว่าง | `OPENSDN_CONFIG_URL` | URL ของ Config API ถ้าว่าง plumb ข้าม stage ของ OpenSDN ทั้ง 3 |
| `--control-url` | ค้นจาก `bgp-router` | `OPENSDN_CONTROL_URLS` | URL ของ control introspect คั่นด้วย comma |
| `--agent-url` | IP ของ `virtual-router` และ `--agent-port` | `OPENSDN_AGENT_URL` | URL ของ agent introspect ใช้เฉพาะ `plumb <vm>` |
| `--control-port` | `8083` | | port ที่ใช้ตอนค้นหา control node เอง |
| `--agent-port` | `8085` | | port ที่ใช้ตอนค้นหา agent เอง |
| `--no-config-token` | ปิด | | ไม่ส่ง Keystone token ไปที่ Config API |
| `--timeout` | `2m0s` | | เวลาสูงสุดของทั้งคำสั่ง |
| `--request-timeout` | `15s` | | เวลาสูงสุดต่อ HTTP call |
| `--insecure` | ปิด | | ไม่ตรวจ TLS certificate |

`plumb doctor` จำกัดเวลาของแต่ละ probe ไว้ที่ 3 วินาที และ probe compute ได้พร้อมกันครั้งละ 16 เครื่อง

## Flag ของ output

`plumb <vm>`, `plumb demo` และ `plumb doctor` รับ flag เหล่านี้:

| Flag | หน้าที่ |
| --- | --- |
| `--json` | พิมพ์ผลเป็น JSON แทน tree |
| `--no-color` | ปิดสี |
| `--no-timings` | ไม่แสดงเวลาของแต่ละ stage |
| `--debug` | พิมพ์ทุก HTTP call ไปที่ stderr และปิด spinner `--dump-http` ทำงานเหมือนกัน |

## Flag ของการบันทึก

`plumb <vm>` เท่านั้นที่รับ 2 flag นี้ และใช้พร้อมกันไม่ได้:

| Flag | หน้าที่ |
| --- | --- |
| `--record DIR` | บันทึกทุก response ลง `DIR` |
| `--replay DIR` | ตอบทุก call จากไฟล์ใน `DIR` โดยไม่ต่อ network |

ถ้าใช้ `--replay` โดยไม่ตั้ง `OS_USERNAME` plumb จะใช้ค่า `replay` แทน เพราะไฟล์บันทึกไม่ขึ้นกับ request body

## ไฟล์บันทึก

`--record` สร้างไฟล์ 1 ไฟล์ต่อ 1 call ชื่อไฟล์มาจาก method, host, path และ query ตาม `httpx.Key` เนื้อหาเป็น HTTP response แบบข้อความ:

```text
HTTP/1.1 200 OK
Content-Type: application/json

{"server": {…}}
```

ไฟล์เก็บเฉพาะ header `Content-Type` และ `X-Subject-Token` และ `--record` เขียน `recorded-token` แทนค่าจริงของ `X-Subject-Token` ไฟล์ไม่เก็บ request จึงไม่มี password lab ของ `plumb demo` ใช้รูปแบบเดียวกันและอยู่ใน `internal/demo/lab`

## Exit code

| Exit code | ความหมาย |
| --- | --- |
| `0` | ไม่มี stage หรือ check ที่ fail อาจมีคำเตือน |
| `1` | มีอย่างน้อย 1 stage หรือ check ที่ fail |
| `2` | คำสั่ง, argument หรือ flag ไม่ถูกต้อง หรือไม่มี credential |

## สถานะของ stage

หัวข้อ `Steps` แสดงสถานะของแต่ละ stage ด้วยเครื่องหมาย 4 แบบ:

| เครื่องหมาย | สถานะใน JSON | ความหมาย |
| --- | --- | --- |
| `✓` | `ok` | API ตอบครบ และข้อมูลระหว่างชั้นตรงกัน |
| `!` | `warn` | API ตอบ แต่ข้อมูลระหว่างชั้นไม่ตรงกัน |
| `✗` | `fail` | call หลักของ stage ไม่สำเร็จ |
| `-` | `skip` | ขาดข้อมูลที่ stage ก่อนหน้าต้องหาให้ |

## JSON ของ trace

`--json` พิมพ์ struct `Trace` ใน `internal/trace/model.go` แต่ละ stage ใน `steps` มี field เหล่านี้:

| Field | ความหมาย |
| --- | --- |
| `name`, `status`, `duration_ms` | ชื่อ สถานะ และเวลาของ stage |
| `error` | ข้อความ error เมื่อ `status` เป็น `fail` หรือเหตุผลเมื่อเป็น `skip` |
| `code`, `hint` | code และสิ่งที่ต้องตรวจต่อ เมื่อ `status` เป็น `fail` และ plumb รู้สาเหตุ |
| `warnings[]` | object ที่มี `code`, `message` และ `hint` |

`plumb explain` รู้จัก `code` ทุกตัวที่ปรากฏใน JSON
