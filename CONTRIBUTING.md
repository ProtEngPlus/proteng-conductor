# Contributing

กติกาเรื่อง branch, commit message, PR และ docs ของทุก repo อยู่ที่ [CONTRIBUTING.md ของ hub](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/CONTRIBUTING.md) หน้านี้มีเฉพาะเรื่องของ proteng-conductor

- แตก branch จาก `dev` และเปิด PR เข้า `dev` push เข้า `dev` จะ deploy ขึ้น dev ทันที และ push เข้า `main` จะ deploy ขึ้น production ทันที
- รัน `make check` ก่อน push ทุกครั้ง คำสั่งนี้ตรวจ gofmt, `go vet` และ `go test`
- ฟังก์ชันใหม่ต้องมี test ใน `_test.go` ของ package เดียวกัน และอยู่ใน commit เดียวกัน
- repo นี้เป็น public ห้ามใส่ credential และรายละเอียดของเครื่องที่ใช้ deploy
- ถ้าเปลี่ยน interface ที่มี `//go:generate mockgen` ให้รัน `make mocks` และ commit mock ที่สร้างใหม่ไปด้วย

## แก้คู่กับ repo อื่น

ของต่อไปนี้ต้องแก้พร้อมกับอีก repo ในงานชุดเดียวกัน และ PR ของทั้งสองฝั่งต้องใส่ `Related: ProtEngPlus/<repo>#<เลข PR>` ถึงกัน รายการเต็มและลำดับการ merge อยู่ใน [CONTRIBUTING ของ hub](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/CONTRIBUTING.md#ของที่ต้องแก้คู่กันข้าม-repo)

- ชื่อ stage ใน `sendJobToPipelineComponent` และ struct ใน `internal/conductor/model.go` ต้องตรงกับที่ ML service ใน proteng-kubeflow รับและส่ง
- path ของ endpoint ที่เปลี่ยนต้องแก้ใน forwarder ของ bff ด้วย
- env ใหม่ใน `.env.example` ต้องเพิ่มใน overlay `dev` และ `production` ของ devops-k8s ด้วย

## hook

`make setup` ติดตั้ง hook ให้ด้วย pre-commit

| ตอน | hook |
| --- | --- |
| commit | `gofmt -l -w` และ `go vet` กับไฟล์ Go ที่ staged |
| push | `go build` และ `go test` เพิ่มจากตอน commit |
| เขียน commit message | ปฏิเสธ message ที่ไม่ตรงกับ Conventional Commits |

ถ้า gofmt แก้ไฟล์ให้ระหว่าง commit ให้ `git add` ไฟล์นั้นซ้ำแล้ว commit อีกครั้ง ถ้าบน Windows `gofmt -l .` แสดงไฟล์ออกมาเต็มไปหมดทั้งที่ไม่ได้แก้ แปลว่า working tree เป็น CRLF ดูวิธีแก้ในหัวข้อ Line endings ของ CONTRIBUTING ของ hub
