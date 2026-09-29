<!-- contentType: How-to · plan: docs/content-plan.md -->

# วิธีเพิ่ม stage ใหม่ให้ plumb

หน้านี้บอกขั้นตอนเพิ่ม stage ที่อ่าน API ใหม่ เช่น Octavia หรือ Cinder แล้วแสดงผลใน tree เหตุผลของโครงสร้างนี้อยู่ใน [plumb ออกแบบอย่างไร](architecture.md)

1. ถ้า API ยังไม่มี client ให้สร้าง package ใหม่ใต้ `apps/cli/internal/` ที่รับ `*httpx.Client` แล้วเรียกผ่าน `httpx.Service`
2. เพิ่ม field ใน `Trace` หรือ `Port` ใน `apps/cli/internal/trace/model.go` สำหรับผลของ stage
3. เขียน method บน `run` ที่มี signature `func(context.Context) error`:
   - ถ้าขาดข้อมูลจาก stage ก่อนหน้า ให้คืนค่าจาก `skip`
   - ถ้า API ตอบแต่ข้อมูลไม่ตรงกับชั้นอื่น ให้เรียก `r.warn` พร้อม code เช่น `r.warn("route-missing", …)`
   - ถ้า call หลักของ stage ไม่สำเร็จ ให้คืน error หรือคืนค่าจาก `fail` ถ้าคุณรู้สาเหตุและมี code ให้
4. ถ้าใช้ code ใหม่ ให้เพิ่ม code, ความหมาย และสิ่งที่ต้องตรวจใน `explanations` ของ `apps/cli/internal/trace/issues.go` แล้วเพิ่มหัวข้อ `` ### `code` `` ใน `docs/troubleshooting.md`
5. เพิ่ม stage ในตาราง `stages` ของ `Run` ใน `apps/cli/internal/trace/trace.go` ตามลำดับที่ข้อมูลต้องใช้
6. แสดงผลใน `apps/cli/internal/render/tree.go`
7. เพิ่มไฟล์ response ของ API ใหม่ใน `apps/cli/internal/demo/lab` ชื่อไฟล์ต้องตรงกับ `httpx.Key` ของ request ถ้าชื่อผิด stage จะ fail ด้วย error `replay: no recording` ตามด้วยชื่อไฟล์ที่ต้องใช้
8. สร้าง golden file ใหม่แล้วตรวจ diff:

   ```sh
   make -C apps/cli golden
   git diff apps/cli/cmd/plumb/testdata/demo.golden
   ```

ถ้า diff แสดงเฉพาะบรรทัดของ stage ใหม่ ให้รัน `make -C apps/cli test` แล้ว commit ไฟล์ทั้งหมด ถ้าอยากให้ผู้ใช้เห็นปัญหาของ stage ใหม่ใน `plumb demo` ให้เพิ่ม scenario ใน `apps/cli/internal/demo/demo.go` และเพิ่มกรณีใน `TestDemoScenarios`
