# proteng-conductor

proteng-conductor เป็นตัวขับ ML pipeline ของ ProtEngPlus ทำหน้าที่เก็บ job, mutation และผลของแต่ละ stage ใน MongoDB ส่งงานของ stage ปัจจุบันไปยัง ML service ผ่าน RabbitMQ แล้วอ่านผลที่ส่งกลับมาเพื่อเลือก stage ถัดไป conductor ถูกเรียกจาก [proteng-bff](https://github.com/ProtEngPlus/proteng-bff) เท่านั้น และตัวมันเองไม่ได้ตรวจ JWT ภาพรวมของระบบอยู่ที่ [manual-guides-2023](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/reference/architecture.md)

## เริ่มใช้

ถ้ายังไม่เคยตั้งเครื่อง ให้ทำตาม [tutorials/01-local-setup.md](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/tutorials/01-local-setup.md) ของ hub ซึ่งตั้งทุก repo พร้อมกัน ถ้าจะตั้งเฉพาะ repo นี้ ให้รันใน Git Bash

```sh
make setup
make -C ../manual-guides-2023 infra-up
make run
```

- `make setup` ติดตั้ง git hook สร้าง `.env.local` จาก `.env.example` (ถ้ามีอยู่แล้วจะไม่ทับ) และดาวน์โหลด Go module ค่าใน `.env.example` ใช้กับ RabbitMQ และ MongoDB ที่ `infra-up` เปิดได้ทันที
- `make run` ผ่านเมื่อเห็น `proteng-conductor is running on :8081`

ต้องใช้ Go 1.22 ขึ้นไป และ `pip install pre-commit`

## คำสั่ง

| คำสั่ง | ทำอะไร |
| --- | --- |
| `make setup` | ติดตั้ง hook, สร้าง `.env.local` และดาวน์โหลด module รันซ้ำได้ |
| `make run` | รันในเครื่องด้วย `ENV=local` |
| `make check` | gofmt, `go vet` และ `go test` |
| `make test` และ `make test-race` | รัน unit test และรันพร้อม race detector ซึ่งต้องมี cgo (บน Windows ต้องมี gcc ใน `PATH`) |
| `make mocks` | สร้าง mock ใหม่จาก comment `go:generate` |
| `make fmt` | จัด format ด้วย gofmt |
| `make build` และ `make docker-build` | compile ทุก package และ build image ในเครื่อง |

CI ของ repo นี้ตอนนี้ตรวจแค่ gofmt และ `go vet` ส่วน test จะถูกเพิ่มเข้า CI ใน PR #124 ให้รัน `make check` ในเครื่องก่อน push เสมอ

## Config

conductor อ่าน `.env.local` เมื่อรันด้วย `ENV=local` ส่วน dev และ production ได้ค่าจาก ConfigMap และ SealedSecret ใน devops-k8s

| ตัวแปร | ค่าตอนรัน local | ใช้ทำอะไร |
| --- | --- | --- |
| `HTTP_PORT` | `8081` | port ที่ conductor ฟัง |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` | broker |
| `JOB_QUEUE` | `job_status_event` | queue ที่ ML service ส่งผลกลับมา |
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB |
| `MONGO_DB` | `proteng_local` | ชื่อ database |
| `STORAGE_EMULATOR_HOST` | ว่าง หรือ `http://localhost:4443` | ถ้าตั้ง library ของ GCS จะอ่าน artifact จาก fake-gcs แทน GCS จริง ปุ่มดาวน์โหลดในหน้าเว็บ local จึงใช้ได้ ใช้ในเครื่องเท่านั้น |
| `PROJECT_ID`, `PRIVATE_KEY_ID`, `PRIVATE_KEY`, `CLIENT_EMAIL`, `CLIENT_ID`, `TOKEN_URI` | ว่างไว้ได้ | service account ของ GCP ใช้ตอนดาวน์โหลด artifact และค้น UniProt ผ่าน `storage/` ถ้าว่างไว้และไม่ได้ตั้ง `STORAGE_EMULATOR_HOST` conductor ยังเริ่มได้ แต่ endpoint เหล่านี้จะ error |

## โครงสร้างโค้ด

| ที่อยู่ | มีอะไร |
| --- | --- |
| `internal/conductor/conductor.go` | logic หลักของ pipeline |
| `internal/conductor/model.go` | struct ของ message ที่รับและส่ง (`Data`, `Payload`, `PipelineRequest`) |
| `internal/rabbitmq/consumer/` | `RunConsumer` อ่าน `JOB_QUEUE` แล้วเรียก `Orchestrate` พร้อม reconnect และ graceful shutdown |
| `internal/rabbitmq/publisher/` | `PublishWithTopic` ส่งงานเข้า exchange `logs_topic` และ `PublishDefaultExchange` ส่งเข้า queue ตรง เช่น `job_status_email_notification` |
| `apis/controllers/`, `apis/routes/` | REST API ของ job, mutation, query result, artifact และ UniProt |
| `models/`, `repositories/` | struct ของข้อมูลใน MongoDB และการอ่านเขียน |
| `storage/` | อ่าน artifact จาก GCS |

### งานเดินอย่างไร

1. bff เรียก `POST /jobs/:id/run` แล้ว `RunJob` ตั้ง job เป็น `ONGOING` จากนั้นเรียก `OrchestrateJob` ซึ่งเตรียม request ของ stage ปัจจุบัน
2. `sendJobToPipelineComponent` ส่ง message เข้า `logs_topic` ด้วย routing key `<stage>.<tool>` โดย `stage_id` 0 ถึง 3 คือ `query`, `evotune`, `fittop` และ `mutation` ตามลำดับ เช่น `query.mmseqs2`
3. ML service ส่งผลกลับเข้า `job_status_event` แล้ว consumer เรียก `Orchestrate` ตัวนี้จะเรียก `updateJobData` เพื่อบันทึกผลและ `run_time` ของ stage นั้น
4. `getNextStage` เลือก state และ stage ถัดไป ถ้า job เป็นแบบ `auto` จะเป็น `ONGOING` และเดินต่อทันที ถ้าไม่ใช่จะเป็น `PENDING` และรอให้ผู้ใช้สั่ง run stage ถัดไป ถ้า evotune ไม่มี lab result job จะเป็น `FAILED` และเมื่อ stage 3 จบ job จะเป็น `COMPLETED`
5. เมื่อ job จบหรือล้มเหลว `sendJobStatusNotificationEmail` ส่ง message เข้า `job_status_email_notification` ให้ user-mgmt ส่งอีเมล

`RunMutation` ใช้กับ mutation ที่สั่งเพิ่มหลังจาก job จบแล้ว ซึ่งต้องเป็น job ที่ `COMPLETED` และอยู่ที่ stage 3

### Mocks

test ใช้ mock จาก `github.com/golang/mock` v1.6.0 ซึ่งสร้างจาก comment `//go:generate mockgen` ใน `internal/rabbitmq/publisher/publisher.go` และ `repositories/*_repository.go` ถ้าเปลี่ยน interface ต้องสร้าง mock ใหม่

```sh
go install github.com/golang/mock/mockgen@v1.6.0
make mocks
```

ต้องใช้ mockgen version นี้ให้ตรงกับ `go.mod` ถ้าใช้ `go.uber.org/mock` ซึ่งเป็นตัวที่มาแทน ไฟล์ที่ได้จะ import คนละ package

## ข้อควรระวัง

- conductor ไม่ตรวจ JWT จึงต้องเรียกผ่าน bff เท่านั้น
- `Orchestrate` ไม่ได้เช็คว่า job ยัง `ONGOING` อยู่ก่อนบันทึกผล ถ้าผลที่ส่งมาช้ามาถึงหลังจากที่ job ถูกตั้งเป็น `FAILED` ไปแล้ว job จะเดินต่อได้ วิธีกู้ job ที่ค้างอยู่ที่ [how-to/recover-job.md](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/how-to/recover-job.md)
- queue ของ ML service เป็นแบบ exclusive และ conductor publish แบบ `mandatory=false` ถ้าตอนนั้นไม่มี service ต่ออยู่ message จะหายไปโดยไม่มี error
- ถ้าชี้ `MONGO_URI` ไปที่ MongoDB ที่ใช้ร่วมกัน ให้ตั้ง `MONGO_DB` เป็นชื่อของตัวเอง เช่น `proteng_<ชื่อ>` ห้ามใช้ `proteng-dev` หรือ `proteng-production`
- deploy conductor ตอนที่มี job รันอยู่อาจทำให้ผลที่กำลังส่งมาหาย ให้ดูก่อนตาม [how-to/deploy-app.md](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/how-to/deploy-app.md)

## Deploy

push เข้า `dev` จะ build image และ deploy ขึ้น dev ส่วน push เข้า `main` จะ deploy ขึ้น production ทั้งสองแบบเกิดขึ้นทันทีทุกครั้งที่ push แม้จะแก้แค่ docs วิธีตรวจและ rollback อยู่ที่ [how-to/deploy-app.md](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/how-to/deploy-app.md)

## ลิงก์

- กติกาการทำงานและ hook ของ repo นี้: [CONTRIBUTING.md](./CONTRIBUTING.md)
- API ทั้งหมดอยู่ใน Swagger ของ bff ที่ <http://localhost:8080/swagger/index.html> เมื่อรัน bff ในเครื่อง
- เอกสารของทั้งระบบ: [manual-guides-2023](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/README.md)
