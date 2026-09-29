<!-- contentType: Tutorial · plan: docs/content-plan.md -->

# รัน plumb ครั้งแรกกับ lab จำลอง

ใน tutorial นี้เราจะ build plumb แล้วใช้ `plumb demo` ไล่ VM `web-01` ใน lab ที่ฝังอยู่ใน binary ตั้งแต่ Keystone ถึง vRouter agent จากนั้นเราจะเปิด scenario ที่ route หายไป 1 จุด แล้วดูว่า plumb ชี้จุดนั้นและบอกสิ่งที่ต้องตรวจต่ออย่างไร ทุกขั้นรันบนเครื่องของคุณโดยไม่ต้องมี cloud หรือ credential

## Build plumb

เราต้องใช้ Go 1.24 ขึ้นไป ที่ root ของ repo ให้ build binary:

```sh
make -C apps/cli build
```

ตรวจว่า binary ทำงาน:

```sh
./apps/cli/bin/plumb version
```

ผลลัพธ์คือ `plumb` ตามด้วย version เช่น `plumb dev`

## รัน lab จำลอง

ตอนนี้ให้รัน demo:

```sh
./apps/cli/bin/plumb demo
```

บรรทัดแรกบอก scenario ที่กำลังรัน ตามด้วย tree ที่ขึ้นต้นด้วย `VM web-01` ส่วนท้ายของผลลัพธ์เป็นแบบนี้:

```text
Steps
  ✓ keystone            0 ms
  ✓ nova                0 ms
  ✓ neutron             0 ms
  ✓ opensdn-config      0 ms
  ✓ control             0 ms
  ✓ vrouter             0 ms

✓ Traced web-01 through 6 of 6 stages in 0 ms
```

เราส่งชื่อ `web-01` ให้ plumb ไม่ใช่ UUID stage `nova` จึงค้น UUID ให้ก่อน ถ้า terminal ของคุณแสดงสีได้ เครื่องหมาย `✓` จะเป็นสีเขียว

## หา port ของ VM ใน tree

ตอนนี้ให้เลื่อนขึ้นไปที่บรรทัด `Port` ซึ่งเป็นส่วนสุดท้ายของ tree ตัวอย่างนี้ตัดบรรทัดยาวด้วย `…`:

```text
└─ Port      9c1e4d2b-7a3f-…  fa:16:3e:5a:12:7c  10.0.1.5
   ├─ Neutron   ACTIVE  vif_type=vrouter  host=compute-02
   ├─ Config    VMI default-domain:admin:9c1e4d2b-…  ✓ same UUID as the port
   ├─ Control   control-01  vn1:vn1.inet.0  10.0.1.5/32 ✓
   └─ vRouter   tap9c1e4d2b-7a ✓ active  vrf vn1:vn1 (index 3)  label 25
```

ใต้ port มีชั้นละ 1 บรรทัด เรียงจาก Neutron ลงไปถึง vRouter agent ให้หาเครื่องหมาย `✓` ใน 3 บรรทัดนี้:

1. บรรทัด `Config`: `✓ same UUID as the port`
2. บรรทัด `Control` ของ `control-01` และ `control-02`: `10.0.1.5/32 ✓`
3. บรรทัด `vRouter`: `✓ active`

หน้า [request ของ VM ผ่านชั้นไหนบ้าง](concepts.md) อธิบายว่าแต่ละบรรทัดเชื่อมกับบรรทัดถัดไปด้วยค่าอะไร

## รันแบบ DevStack

DevStack ใช้ OVN จึงไม่มี Config API ของ OpenSDN ให้เรียก scenario `devstack` รัน lab เดิมโดยไม่มี Config API:

```sh
./apps/cli/bin/plumb demo devstack
```

ส่วนท้ายเปลี่ยนเป็นแบบนี้:

```text
  - opensdn-config   skipped: ports use vif_type ovs, so this cloud does …
  - control          skipped: needs opensdn-config
  - vrouter          skipped: needs opensdn-config

✓ Traced web-01 through 3 of 6 stages in 0 ms
  Skipped opensdn-config: ports use vif_type ovs, so this cloud does not …
```

port ใน scenario นี้มี `vif_type` เป็น `ovs` plumb จึงบอกว่า cloud นี้ไม่ได้ใช้ OpenSDN แล้วข้าม stage ของ OpenSDN ทั้ง 3 ส่วน 3 stage แรกยังทำงานครบ

## ทำให้ route หายจาก control node 1 ตัว

ต่อไปเราจะเปิด scenario ที่ `control-02` ไม่มี route ของ VM:

```sh
./apps/cli/bin/plumb demo missing-route
```

บรรทัด `Control` ของ `control-02` ใน tree กลายเป็น `10.0.1.5/32 ✗ missing` และผลลัพธ์จบด้วย 3 บรรทัดนี้:

```text
! Traced web-01 through 6 of 6 stages in 0 ms, 1 issue
  Hint  Check that the port's vRouter line shows active, then check …
  More  plumb explain route-missing
```

บรรทัด `Hint` บอกสิ่งที่ต้องตรวจต่อ และบรรทัด `More` บอกคำสั่งที่อธิบายปัญหานี้ ให้รันคำสั่งนั้น:

```sh
./apps/cli/bin/plumb explain route-missing
```

plumb พิมพ์ความหมายของ `route-missing`, สิ่งที่ต้องตรวจ และหัวข้อใน [คำเตือนแต่ละข้อของ plumb หมายถึงอะไร](troubleshooting.md)

## ดู scenario อื่น

ขั้นสุดท้าย ให้ดูรายการ scenario ทั้งหมด:

```sh
./apps/cli/bin/plumb demo --list
```

แต่ละ scenario ทำให้ชั้นหนึ่งพัง ลองรัน `agent-down` แล้วสังเกตว่าเครื่องหมายหน้าบรรทัดสุดท้ายเปลี่ยนเป็น `✗` และคำสั่งจบด้วย exit code `1` เพราะ stage `vrouter` เรียก API ไม่สำเร็จ ส่วน `missing-route` จบด้วย exit code `0` เพราะ API ทุกตัวตอบปกติ มีเพียงข้อมูลที่ไม่ตรงกัน

## อ่านต่อ

ตอนนี้เรารัน plumb อ่าน tree และตามคำแนะนำจาก hint ได้แล้ว หน้าถัดไปขึ้นกับงานที่คุณจะทำ:

- [วิธีใช้ plumb กับ DevStack และ lab OpenSDN](run-against-a-lab.md)
- [คำเตือนแต่ละข้อของ plumb หมายถึงอะไร](troubleshooting.md)
