<!-- contentType: How-to · plan: docs/content-plan.md -->

# วิธีใช้ plumb กับ DevStack และ lab OpenSDN

หน้านี้บอกขั้นตอนติดตั้ง plumb บน server, ตรวจว่า server เข้าถึงทุกชั้นได้ แล้วรันกับ cloud จริง 2 แบบ คือ DevStack ที่ใช้ OVN และ lab ที่ใช้ OpenSDN flag และ variable ทั้งหมดอยู่ใน [ตัวเลือกของ plumb](cli-reference.md)

## ติดตั้งบน server

plumb ต้องเข้าถึง introspect ของ vRouter agent บน compute ทุกเครื่อง จึงควรรันจาก bastion หรือ controller ที่อยู่ใน management network

1. Build binary แบบ static บนเครื่องของคุณ:

   ```sh
   make -C apps/cli dist
   ```

   คำสั่งนี้สร้าง `apps/cli/dist/plumb-linux-amd64`, `apps/cli/dist/plumb-linux-arm64` และ `apps/cli/dist/plumb-darwin-arm64` ถ้าต้องการให้ binary แจ้งเมื่อมี version ใหม่ ให้ส่ง URL ที่ตอบ release ล่าสุด:

   ```sh
   make -C apps/cli dist VERSION=v0.1.0 \
   	UPDATE_URL=https://api.github.com/repos/your_org/plumb/releases/latest
   ```

2. คัดลอก binary ที่ตรงกับ server:

   ```sh
   scp apps/cli/dist/plumb-linux-amd64 your_user@your_bastion:~/bin/plumb
   ```

binary ไม่ต้องใช้ library อื่น server จึงไม่ต้องติดตั้ง Go

## ตรวจว่า server เข้าถึงทุกชั้น

1. บน server ให้ source openrc ของ project ที่ VM อยู่:

   ```sh
   source ~/admin-openrc
   ```

2. ถ้า cloud ใช้ OpenSDN ให้ link cloud นี้กับ Config API:

   ```sh
   plumb link --config-url http://config_node_ip:8082
   ```

   plumb จำ URL นี้ไว้ใน `~/.config/plumb/config.json` คู่กับ `OS_AUTH_URL` ของ cloud คำสั่ง `trace` และ `doctor` จึงใช้ URL นี้เองทุกครั้งที่ source openrc ของ cloud นี้

3. รัน `doctor`:

   ```sh
   plumb doctor
   ```

`doctor` ขอ token จาก Keystone แล้วเรียก endpoint ทุกตัวที่ trace ต้องใช้ รวมถึง vRouter agent บน compute ทุกเครื่อง บรรทัดสุดท้ายบอกว่า trace จาก server นี้ไปได้ถึงชั้นไหน compute ที่เข้าไม่ถึงแสดงเป็นรายการใต้บรรทัด `vrouter`

ถ้า `doctor` จบด้วย `✗` ให้ทำตามบรรทัด `Hint` ก่อนรัน trace

## รันกับ DevStack

DevStack ใช้ OVN จึงไม่ต้อง link Config API ให้ส่งชื่อหรือ UUID ของ VM:

```sh
plumb your_vm_name
```

ใน tree บรรทัด `Neutron` ของ port ต้องแสดง `vif_type=ovs` ถ้าแสดง `vif_type=vrouter` แปลว่า cloud นี้ใช้ OpenSDN ให้ทำตามหัวข้อถัดไป

ถ้ามี VM หลายเครื่องที่ใช้ชื่อเดียวกัน plumb จะแสดง UUID ของทุกเครื่อง ให้รันใหม่ด้วย UUID ที่ต้องการ

## รันกับ lab OpenSDN

หลัง link cloud ตามหัวข้อ "ตรวจว่า server เข้าถึงทุกชั้น" แล้ว ให้รัน:

```sh
plumb your_vm_name
```

plumb หา control node จาก object `bgp-router` และหา vRouter agent จาก object `virtual-router` ใน Config API

ถ้าไม่แน่ใจว่า plumb ใช้ URL ไหน ให้รัน `plumb whoami` ซึ่งแสดง URL และบอกว่ามาจาก `plumb link`, environment variable หรือ flag

ถ้า stage `control` หรือ `vrouter` fail เพราะ server เข้า IP ที่ config บันทึกไว้ไม่ได้ ให้ระบุ URL เอง:

```sh
plumb your_vm_name \
	--control-url http://control_1_ip:8083,http://control_2_ip:8083 \
	--agent-url http://compute_mgmt_ip:8085
```

ถ้าไม่ต้องการส่ง Keystone token ไปที่ Config API ให้เพิ่ม `--no-config-token`

## ตรวจว่า VM หนึ่งส่ง traffic ถึงอีกเครื่องได้

ถ้าได้รับแจ้งว่า VM A คุยกับ VM B ไม่ได้ ให้ส่งทั้งคู่และ protocol ที่ใช้:

```sh
plumb path web-01 db-01 --port 5432
```

plumb ตรวจ port, router, security group ทั้ง 2 ฝั่ง และบน OpenSDN ตรวจ route กับ next hop ใน VRF ของต้นทาง บรรทัดสุดท้ายบอก check แรกที่ไม่ผ่าน และบรรทัด `Hint` บอกคำสั่งที่แก้ได้ เช่น `openstack security group rule create …` ถ้าใส่ `--port` โดยไม่ใส่ `--proto` plumb จะตรวจ TCP ถ้าไม่ใส่ทั้งคู่จะตรวจ ICMP แบบ `ping`

บน DevStack ที่ใช้ OVN check `route` และ `next-hop` จะถูกข้าม บรรทัดสุดท้ายจึงบอกว่า Neutron ปล่อย traffic ไม่ได้ยืนยันว่า datapath ส่งได้จริง

## บันทึก lab ไว้เปิดแบบ offline

1. รันกับ lab จริงพร้อมบันทึก response:

   ```sh
   plumb your_vm_name --record lab-recordings/web-01
   ```

2. คัดลอก directory กลับมาที่เครื่องของคุณ แล้วเปิดผล โดยใช้ `OS_AUTH_URL` และ flag ชุดเดียวกับตอนบันทึก:

   ```sh
   plumb your_vm_name --replay lab-recordings/web-01
   ```

ไฟล์ใน `lab-recordings/` มี IP ภายในและชื่อ project ของ lab ห้าม commit ไฟล์เหล่านี้ `.gitignore` ของ repo ไม่รวม directory นี้อยู่แล้ว

## เทียบชื่อ field ของ introspect กับ lab

ชื่อ request และ field ของ introspect ต่างกันได้ตาม release ของ OpenSDN ให้ทำขั้นตอนนี้ครั้งแรกที่ใช้ plumb กับ lab ใหม่

1. บันทึก lab ตามหัวข้อก่อนหน้า
2. เปิดไฟล์ที่ชื่อขึ้นต้นด้วย `GET_` ตามด้วย IP ของ control node หรือ compute
3. เทียบชื่อ element ในไฟล์กับตารางใน [API ที่ plumb เรียกในแต่ละชั้น](api-reference.md)
4. ถ้าชื่อไม่ตรง ให้แก้ชื่อใน `apps/cli/internal/opensdn/control/control.go` หรือ `apps/cli/internal/opensdn/agent/agent.go`

อีกทางหนึ่งคือเปิด `http://control_node_ip:8083/` ใน browser หน้านั้นแสดงรายการ request ทั้งหมดของ process

## ส่งผลให้โปรแกรมอื่น

ถ้าต้องการใช้ผลใน script ให้ใช้ `--json` แล้วอ่านด้วย `jq` คำสั่งนี้แสดง code ของคำเตือนทุกตัว:

```sh
plumb your_vm_name --json | jq -r '.steps[].warnings[]?.code'
```

Script ตรวจ exit code ได้ `1` แปลว่ามีอย่างน้อย 1 stage ที่ fail
