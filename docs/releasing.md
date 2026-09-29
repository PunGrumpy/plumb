<!-- contentType: How-to · plan: docs/content-plan.md -->

# วิธีออก release ของ plumb

หน้านี้บอกขั้นตอนตั้งแต่บันทึกการเปลี่ยนแปลงจนได้ binary บน GitHub Release plumb ใช้ Changesets จัดการ version และ `apps/cli/CHANGELOG.md` และใช้ GoReleaser build binary ทั้งสองทำงานใน `.github/workflows/release.yml`

Version ของ plumb อยู่ใน `apps/cli/package.json` ไฟล์นี้เป็น `private` และมีไว้ให้ Changesets ใช้เท่านั้น ตัว plumb ไม่ได้ใช้ Node

## เพิ่ม changeset ใน PR

ทุก PR ที่ผู้ใช้ควรรู้ ต้องมี changeset 1 ไฟล์

1. ติดตั้ง Changesets ครั้งแรกบนเครื่องด้วย [Bun](https://bun.sh):

   ```sh
   bun install
   ```

2. สร้าง changeset แล้วเลือกระดับของการเปลี่ยนแปลง:

   ```sh
   bun changeset
   ```

3. เขียนสรุปสำหรับผู้ใช้ ข้อความนี้จะอยู่ใน `CHANGELOG.md` และ release notes ตามที่เขียน
4. Commit ไฟล์ใหม่ใน `.changeset/` ไปพร้อมกับโค้ด

เลือกระดับตามตารางนี้:

| ระดับ | ใช้เมื่อ | ก่อน 1.0.0 |
| --- | --- | --- |
| `patch` | แก้ bug หรือข้อความ โดยไม่เพิ่มความสามารถ | 0.1.0 เป็น 0.1.1 |
| `minor` | เพิ่มคำสั่ง, flag หรือ check ใหม่ | 0.1.0 เป็น 0.2.0 |
| `major` | ลบหรือเปลี่ยนคำสั่ง, flag หรือ field ใน JSON ที่คนอื่นใช้อยู่ | 0.1.0 เป็น 1.0.0 |

PR ที่ผู้ใช้ไม่เห็นผล เช่นแก้ test หรือ CI ไม่ต้องมี changeset

## ออก release

1. Merge PR ที่มี changeset เข้า `main`
2. Workflow เปิดหรืออัปเดต PR ชื่อ "chore: version packages" ซึ่ง bump version ใน `apps/cli/package.json` เขียน `apps/cli/CHANGELOG.md` และลบไฟล์ changeset ที่ใช้แล้ว
3. ตรวจ `apps/cli/CHANGELOG.md` ใน PR นั้น ถ้าต้องการแก้ข้อความ ให้แก้ใน PR ได้เลย
4. Merge PR "chore: version packages" เมื่อพร้อมออก release

PR "chore: version packages" รวม changeset ทุกไฟล์ที่ merge เข้ามาจนถึงตอนนั้น ถ้ายังไม่อยากออก release ให้เปิด PR นั้นค้างไว้

หลัง merge workflow สร้าง tag `vx.y.z` จาก version ใน `apps/cli/package.json` แล้ว GoReleaser build binary สำหรับ Linux และ macOS ทั้ง `amd64` และ `arm64` สร้าง GitHub Release ที่ใช้ส่วนของ version นั้นใน `apps/cli/CHANGELOG.md` เป็น release notes และแนบไฟล์ `.tar.gz` กับ `checksums.txt`

## สิ่งที่ release ใส่ใน binary

GoReleaser ใส่ค่าเหล่านี้ตอน build ผ่าน `-ldflags`:

- `main.version` เป็นชื่อ tag เช่น `v0.2.0` ซึ่ง `plumb version` แสดง
- `main.updateURL` เป็น `https://api.github.com/repos/<owner>/<repo>/releases/latest` ของ repo ที่ build ทำให้ binary แจ้งเมื่อมี release ใหม่

ถ้า build เองด้วย `make -C apps/cli dist` โดยไม่ตั้ง `UPDATE_URL` binary จะไม่เช็ก version ใหม่

## ตรวจ config ก่อน push

ถ้าแก้ `.goreleaser.yaml` ให้ตรวจและลอง build บนเครื่องก่อน:

```sh
goreleaser check
goreleaser build --snapshot --clean --single-target
```

ถ้าแก้ไฟล์ใน `.github/workflows/` ให้ตรวจด้วย `actionlint` ถ้าแก้ `.changeset/config.json` ให้ดูผลด้วย `bun changeset status`
