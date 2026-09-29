<!-- contentType: How-to · plan: docs/content-plan.md -->

# วิธีออก release ของ plumb

หน้านี้บอกขั้นตอนตั้งแต่เขียน commit จนได้ binary บน GitHub Release plumb ใช้ release-please จัดการ version และ `CHANGELOG.md` และใช้ GoReleaser build binary ทั้งสองทำงานใน `.github/workflows/release.yml`

## เขียน commit ตาม Conventional Commits

release-please อ่าน type ของ commit ที่ merge เข้า `main` เพื่อเลือก version ถัดไป:

| Commit | ผลกับ version ก่อน 1.0.0 | ผลกับ version ตั้งแต่ 1.0.0 |
| --- | --- | --- |
| `fix: …` | patch เช่น 0.1.0 เป็น 0.1.1 | patch |
| `feat: …` | minor เช่น 0.1.0 เป็น 0.2.0 | minor |
| `feat!: …` หรือมี `BREAKING CHANGE:` ใน body | minor | major |
| `docs:`, `chore:`, `ci:`, `test:` | ไม่ออก release | ไม่ออก release |

ใส่ scope ได้ เช่น `feat(path): …` scope ไม่มีผลกับ version

## ออก release

1. Merge PR เข้า `main` ตามปกติ
2. release-please เปิดหรืออัปเดต PR ชื่อ `chore(main): release x.y.z` ซึ่งแก้ `CHANGELOG.md` และ `.release-please-manifest.json`
3. ตรวจ changelog ใน PR นั้น ถ้าต้องการแก้ข้อความ ให้แก้ใน PR ได้เลย
4. Merge PR ของ release

หลัง merge release-please สร้าง tag `vx.y.z` และ GitHub Release จากนั้น GoReleaser build binary สำหรับ Linux และ macOS ทั้ง `amd64` และ `arm64` แล้วแนบไฟล์ `.tar.gz` กับ `checksums.txt` เข้า release นั้น

## สิ่งที่ release ใส่ใน binary

GoReleaser ใส่ค่าเหล่านี้ตอน build ผ่าน `-ldflags`:

- `main.version` เป็นชื่อ tag เช่น `v0.2.0` ซึ่ง `plumb version` แสดง
- `main.updateURL` เป็น `https://api.github.com/repos/<owner>/<repo>/releases/latest` ของ repo ที่ build ทำให้ binary แจ้งเมื่อมี release ใหม่

ถ้า build เองด้วย `make dist` โดยไม่ตั้ง `UPDATE_URL` binary จะไม่เช็ก version ใหม่

## ตรวจ config ก่อน push

ถ้าแก้ `.goreleaser.yaml` ให้ตรวจและลอง build บนเครื่องก่อน:

```sh
goreleaser check
goreleaser build --snapshot --clean --single-target
```

ถ้าแก้ไฟล์ใน `.github/workflows/` ให้ตรวจด้วย `actionlint`
