# Setup

Setup ทั้งระบบครั้งแรกดูที่ [Guidebook](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/README.md) ไฟล์นี้มีแค่รายละเอียดเฉพาะของ `proteng-conductor`

## รันบนเครื่อง

ขั้นตอนหลัก (`cp .env.example .env.local` → `go mod tidy` → `./run.sh` และต้องมี RabbitMQ กับ
MongoDB local) อยู่ใน Guidebook §4.3–4.4 `.env.example` มี default local ครบแล้ว (`RABBITMQ_URL`,
`MONGO_URI`, `MONGO_DB`) เติมเองแค่ GCP service-account block ถ้าจะใช้ GCS จริง

ที่ต้องรู้เพิ่มเฉพาะ conductor:

- ไม่อยากรัน Mongo local จะชี้ `MONGO_URI` ใน `.env.local` (ไม่ใช่ `.env.example`) ไป shared
  cluster ก็ได้ ขอ connection string จาก maintainer แล้วตั้ง `MONGO_DB` เป็นชื่อตัวเอง (เช่น
  `proteng_<ชื่อคุณ>`) อย่าใช้ `proteng-dev` / `proteng-production`
- `./run.sh` แค่ตั้ง `ENV=local` แล้ว `go run main.go` (ไม่มี `.env.dev` แล้ว)
- เสร็จเมื่อ terminal พิมพ์ `proteng-conductor is running on :8081` (หรือ `HTTP_PORT` ที่ตั้ง) แล้วไม่ crash

## Format & lint

`gofmt` autofix ตอน save/commit, `go vet` รายงานอย่างเดียวต้องแก้เอง รันมือทั้ง repo:

```sh
gofmt -l -w .
go vet ./...
```

ทั้งคู่รันเป็น pre-commit hook ให้อัตโนมัติ (ดู [CONTRIBUTING.md](./CONTRIBUTING.md)) และรันใน CI
ทุก push ด้วย

## สร้าง mock

ต้องมี `mockgen` ใส่ comment `go:generate` เหนือ interface ที่จะ mock (ดูตัวอย่างใน `repositories/`)
แล้ว:

```sh
go generate ./...
```

## API docs

conductor ถูกเรียกจาก proteng-bff เท่านั้น (frontend ไม่เรียกตรง) API docs อยู่ที่ Swagger ของ
**bff** ไม่ใช่ที่นี่: `http://localhost:8080/swagger/index.html` (ดู `proteng-bff/SETUP.md`)

## Build (ถ้าจะทดสอบ deploy)

env var ไม่ถูก bake เข้า image ส่งตอน run:

```sh
docker build -t proteng-conductor .
docker run -d --name proteng-conductor --env-file .env.local -p 8081:8081 proteng-conductor
```
