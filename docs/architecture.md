<!-- contentType: Conceptual · plan: docs/content-plan.md -->

# plumb ออกแบบอย่างไร และต่อยอดได้ตรงไหน

หน้านี้อธิบายโครงสร้าง package ของ plumb, การตัดสินใจ 7 ข้อที่กำหนดรูปของโค้ด และเหตุผลของแต่ละข้อ ขั้นตอนเพิ่ม stage อยู่ใน [วิธีเพิ่ม stage ใหม่ให้ plumb](add-a-stage.md)

## โครงสร้าง package

โค้ด Go อยู่ใน `apps/cli/` ซึ่งเป็น module `github.com/PunGrumpy/plumb/apps/cli` โค้ดแบ่งเป็น 4 กลุ่ม คือ client ของแต่ละ API, stage ที่ร้อยข้อมูลเข้าด้วยกัน, ส่วนแสดงผล และคำสั่งที่ช่วยผู้ใช้เริ่มต้น:

```text
apps/cli/
  cmd/plumb/              คำสั่ง, flag, transport และ exit code
  internal/httpx/         HTTP client, --debug, --record, --replay
  internal/keystone/      token และ service catalog
  internal/nova/          server และ os-interface
  internal/glance/        ชื่อ image
  internal/neutron/       port, network, subnet, security group, FIP
  internal/opensdn/
    config/               Config API (8082)
    sandesh/              parser ของ introspect แบบ generic
    control/              control node introspect (8083)
    agent/                vRouter agent introspect (8085)
  internal/trace/         struct Trace, 6 stage และ issue code
  internal/render/        tree, verdict และ JSON
  internal/ui/            สี และ spinner
  internal/demo/          lab ที่ฝังใน binary และ scenario
  internal/doctor/        ตรวจการเข้าถึงทุกชั้น
```

Client แต่ละตัวรู้จักเฉพาะ API ของตัวเอง และไม่ import กันเอง มีเพียง `apps/cli/internal/trace` ที่รู้ว่าค่าจากชั้นไหนต้องส่งต่อไปชั้นไหน

## การตัดสินใจ 7 ข้อและสิ่งที่ต้องยอมแลก

ทั้ง 7 ข้อเลือกให้เห็นข้อมูลของแต่ละชั้นชัดที่สุด แม้โค้ดจะยาวขึ้น โปรเจกต์นี้มีไว้เรียนรู้ระบบ ความชัดเจนจึงสำคัญกว่าจำนวนบรรทัด

### เขียน request เองด้วย `net/http` แทน gophercloud

plumb สร้าง request ทุกตัวด้วย standard library เป้าหมายของโปรเจกต์คือให้เห็น URL, header และ microversion ของแต่ละชั้น gophercloud ซ่อนทั้ง 3 อย่างไว้ใน library

สิ่งที่ต้องยอมแลกคือโค้ดต้องทำงานที่ library เคยทำให้ เช่นเติม `/v3` ท้าย `OS_AUTH_URL` และเติม `/v2.0` ท้าย endpoint ของ Neutron งานแบบนี้มีไม่กี่จุด จึงคุ้มกับการได้เห็น request จริง

### ทุก call ผ่าน `httpx.Client` จุดเดียว

Client ทุกตัวเรียก HTTP ผ่าน `httpx.Client` เพราะเหตุนี้ 3 feature ต่อไปนี้จึงใช้ได้กับทุกชั้นโดยไม่ต้องแก้ client:

- `--debug` log ทุก call
- `--record` เป็น `http.RoundTripper` ที่บันทึก response ลงไฟล์
- `--replay` เป็น `http.RoundTripper` ที่ตอบจากไฟล์

ไฟล์บันทึกเป็น HTTP response แบบข้อความ คนเขียนหรือแก้ไฟล์ด้วยมือได้ และ `http.ReadResponse` อ่านไฟล์กลับได้ ผลที่ได้คือ lab ใน `apps/cli/internal/demo/lab` ใช้ได้ทั้งใน `plumb demo` และใน test โดยไม่ต้องมี mock server

### `Trace` เป็นข้อตกลงเดียวระหว่าง stage กับการแสดงผล

Stage เขียนผลลง struct `Trace` ใน `apps/cli/internal/trace/model.go` และ renderer อ่านจาก struct นั้นเท่านั้น tree, `--json` และหน้าเว็บในอนาคตจึงแสดงข้อมูลชุดเดียวกัน

`Trace` เก็บผลแยกตาม port เพราะ UUID ของ port เป็นค่าเดียวที่ใช้ได้ตั้งแต่ Nova ถึง vRouter agent VM ที่มี 2 port จึงมี `Port` 2 ตัว และแต่ละตัวเก็บผลของทุกชั้น

### Stage ที่มีปัญหาไม่หยุด stage ถัดไป

แต่ละ stage จบด้วยสถานะ `ok`, `warn`, `fail` หรือ `skip` แล้ว stage ถัดไปทำงานต่อ ความหมายของแต่ละสถานะอยู่ใน [ตัวเลือกของ plumb](cli-reference.md)

ทางเลือกนี้ทำให้ผลลัพธ์บอกเสมอว่าข้อมูลไปถึงชั้นไหนและหยุดที่ชั้นไหน ซึ่งเป็นคำถามที่คนเปิด plumb ต้องการคำตอบ ถ้าหยุดที่ error แรก ผู้ใช้จะเห็นเฉพาะชั้นที่พังและไม่เห็นว่าชั้นอื่นยังปกติ

Stage `neutron` ค้น port ด้วย `device_id` และไม่ใช้ผลของ Nova ถ้า stage `nova` fail stage ถัดไปจึงยังมี port ให้ใช้

### Parse Sandesh เป็น tree แทน struct

Introspect ตอบเป็น XML ของ Sandesh และชื่อ field เปลี่ยนได้ตาม release package `sandesh` จึง parse XML เป็น tree ของ `Node` แล้ว client อ่าน field ตามชื่อด้วย `Str`, `Int` และ `Strings`

ถ้า release ของคุณไม่มี field ใด ค่าที่อ่านได้จะว่าง ไม่ใช่ error การแก้ชื่อ field ทำใน `apps/cli/internal/opensdn/control` หรือ `apps/cli/internal/opensdn/agent` โดยไม่แตะ parser ข้อเสียคือ compiler ไม่เตือนเมื่อชื่อ field ผิด test ของ lab จำลองจึงต้องครอบคลุมทุก field ที่ tree แสดง

### อ่านอย่างเดียว และไม่ส่ง token ไปที่ introspect

plumb ส่ง `POST` ครั้งเดียวไปที่ Keystone เพื่อขอ token call อื่นเป็น `GET` ทั้งหมด plumb ส่ง token ไปที่ Nova, Glance, Neutron และ Config API เท่านั้น introspect ไม่มี auth และรับ request ผ่าน HTTP ที่ไม่เข้ารหัส การส่ง token ไปที่ introspect จึงเสี่ยงโดยไม่ได้ประโยชน์

### ทุกปัญหามี code, hint และคำสั่งที่อธิบายต่อ

คำเตือนและ error ที่ plumb รู้สาเหตุมี code คงที่ เช่น `route-missing` แต่ละ code มีความหมายและสิ่งที่ต้องตรวจต่อใน `apps/cli/internal/trace/issues.go` ข้อมูลชุดนั้นใช้ใน 4 ที่ คือ บรรทัด `Hint` ท้าย tree, คำสั่ง `plumb explain`, ฟิลด์ `code` และ `hint` ใน JSON และหัวข้อใน `docs/troubleshooting.md`

ผมเลือกทางนี้เพราะคนที่เปิด plumb ตอนระบบพังต้องการรู้ 2 อย่าง คืออะไรพังและต้องทำอะไรต่อ ข้อความ error ของ Go บอกได้เฉพาะอย่างแรก ราคาที่ต้องจ่ายคือ code ใหม่ทุกตัวต้องมีคำอธิบายและหัวข้อในเอกสาร test `TestEveryCodeIsDocumented` จึง fail ถ้าหัวข้อหายไป

`plumb demo` ใช้แนวคิดเดียวกัน ผู้ใช้ครั้งแรกเห็นผลลัพธ์ที่ถูกต้องได้ก่อนมี credential และแต่ละ scenario แสดงหน้าตาของปัญหา 1 แบบ คนที่เคยเห็น `plumb demo missing-route` จะจำผลลัพธ์แบบเดียวกันได้เมื่อเจอบน cloud จริง

## โปรเจกต์เสริมที่ใช้โค้ดชุดนี้ต่อได้

โปรเจกต์เสริม 3 ตัวใช้ package ของ plumb ได้โดยตรง:

- หน้าเว็บ object graph ของ OpenSDN อ่านผลของ `plumb --json` หรือเรียก `apps/cli/internal/opensdn/config` เอง `Ref` ทุกตัวมี `to` และ `uuid` ของปลายทาง จึงสร้าง edge ได้โดยไม่ต้อง `GET` เพิ่ม
- ตัวตรวจ drift ระหว่าง Neutron กับ OpenSDN เพิ่ม method ที่ list object ใน client ของ `neutron` และ `config` แล้วเทียบ UUID ของ port กับ UUID ของ VMI
- Control plane จำลองใช้ struct ใน `apps/cli/internal/trace/model.go` เป็นต้นแบบของ message ระหว่าง controller กับ agent จำลอง
