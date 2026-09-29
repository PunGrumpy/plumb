<!-- contentType: Reference -->

# แผนเนื้อหาเอกสารของ plumb

หน้านี้เป็นแผนของเอกสารทุกหน้าใน repo นี้ บอกว่าแต่ละหน้ามีไว้ทำอะไร เขียนให้ใคร และยังมีคำถามอะไรค้างอยู่

## ภาพรวมและผู้อ่าน

เอกสารชุดนี้เขียนให้ผู้อ่าน 2 กลุ่ม:

- ผู้ฝึกงาน OJT ที่อยากเข้าใจว่า VM หนึ่งเครื่องผ่าน OpenStack และ OpenSDN ชั้นไหนบ้าง เอกสารนี้ใช้ประกอบ OJT ข้อ 11 เรื่อง DevStack, ข้อ 13 เรื่อง API Tutorial OpenSDN และ OpenStack และข้อ 19 เรื่อง Python Clean OpenStack
- ผู้ดูแล cloud ที่อยากรู้ว่า chain ของ VM หยุดที่ชั้นไหน โดยไม่ต้องเปิด introspect ทีละหน้า

## เป้าหมายของผู้อ่าน

หลังอ่านครบ ผู้อ่านควรทำสิ่งต่อไปนี้ได้:

1. รัน plumb กับ lab จำลอง, DevStack และ lab OpenSDN
2. อธิบายว่า UUID และชื่อ object ตัวไหนเชื่อมแต่ละชั้นเข้าด้วยกัน
3. อธิบายว่าทำไม Config API แยกจาก control node
4. แปลความหมายคำเตือนแต่ละข้อ แล้วเลือก API ที่จะตรวจต่อ
5. เพิ่ม stage ใหม่หรือเขียน UI ใหม่ที่อ่าน JSON ของ plumb

## หน้าเอกสารแต่ละหน้า

แต่ละหน้าทำงานเดียวตาม content type ของตัวเอง:

| หน้า | Content type | เป้าหมาย |
| --- | --- | --- |
| [README](../README.md) | Landing | เลือกหน้าที่ต้องอ่านต่อ |
| [รันครั้งแรกกับ lab จำลอง](quickstart.md) | Tutorial | รัน plumb และอ่านผลลัพธ์ |
| [ใช้กับ DevStack และ lab จริง](run-against-a-lab.md) | How-to | รันกับ cloud จริงและบันทึก lab |
| [เพิ่ม stage ใหม่](add-a-stage.md) | How-to | เพิ่ม API ใหม่เข้า trace |
| [ตัวเลือกของ plumb](cli-reference.md) | Reference | ค้น flag, variable และ exit code |
| [ชั้นต่าง ๆ เชื่อมกันอย่างไร](concepts.md) | Conceptual | อธิบายแต่ละชั้นให้คนอื่นฟังได้ |
| [การออกแบบ](architecture.md) | Conceptual | อธิบายเหตุผลของโครงสร้างโค้ด |
| [API ที่เรียก](api-reference.md) | Reference | ตรวจ endpoint และ field |
| [แปลความหมายคำเตือน](troubleshooting.md) | Troubleshooting | หาสาเหตุเมื่อ chain หยุด |

## คำถามที่ยังเปิดอยู่

ข้อเหล่านี้ต้องยืนยันกับ lab OpenSDN จริงก่อนใช้ plumb บน environment ของทีม:

- ชื่อ parameter และ field ของ Sandesh บน release ที่ทีมใช้ตรงกับใน [API ที่เรียก](api-reference.md) หรือไม่
- Config API บน lab เปิด keystone auth หรือไม่
- ใครอนุญาตให้เครื่องที่รัน plumb เข้า port 8082, 8083 และ 8085 ได้ คนที่ต้องถามคือพี่หมู
- ข้อมูลใน `internal/demo/lab` เขียนขึ้นเองตามรูปแบบของ API ยังไม่ได้บันทึกจาก lab จริง
