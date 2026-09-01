# proteng-conductor

Job orchestrator (Gin + MongoDB) หัวใจของ ProtEngPlus จัดการ state ของ job/pipeline รับ event ตอน stage เสร็จจาก RabbitMQ แล้วสั่ง stage ถัดไปไปที่ microservice ของ [proteng-kubeflow](https://github.com/ProtEngPlus/proteng-kubeflow) ถูกเรียกจาก [proteng-bff](https://github.com/ProtEngPlus/proteng-bff)

วิธีรัน local ดู [SETUP.md](./SETUP.md) กติกา commit กับ pre-commit hook ดู [CONTRIBUTING.md](./CONTRIBUTING.md)

เพิ่งเริ่มกับ ProtEngPlus? เริ่มที่ [Guidebook](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/README.md) ก่อน
