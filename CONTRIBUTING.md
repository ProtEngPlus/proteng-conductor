# Contributing

กติกา commit message / branch / PR กับวิธีติดตั้ง pre-commit ของทุก repo ProtEngPlus เขียนรวมไว้ที่
[manual-guides-2023/CONTRIBUTING.md](https://github.com/ProtEngPlus/manual-guides-2023/blob/main/CONTRIBUTING.md)
repo นี้เก็บแค่ hook เฉพาะของตัวเอง

## Pre-commit hooks

- **pre-commit**: `gofmt -l -w` + `go vet` กับไฟล์ Go ที่ staged
- **pre-push**: รัน `go build` + `go test` เพิ่ม
- **commit-msg**: ปฏิเสธ commit message ที่ผิดฟอร์แมต Conventional Commits

`gofmt` + `go vet` รันใน CI (`.github/workflows/test-build-dev.yaml`) ทุก push ด้วย ข้าม hook local
ด้วย `--no-verify` ก็แค่ให้ CI จับแทน

รันมือทั้งหมด: `pre-commit run --all-files`
