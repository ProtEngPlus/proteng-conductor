# Setup

## Run locally

1. **Copy the env file**

   ```sh
   cp .env.example .env.local
   ```

   `.env.example` already has working local defaults for `RABBITMQ_URL`
   (`amqp://guest:guest@localhost:5672/`) and `MONGO_URI`
   (`mongodb://localhost:27017`). Only the GCP service-account block
   (`PROJECT_ID` … `TOKEN_URI`) needs real values, and only if you exercise GCS
   artifact storage - get those from a maintainer.

   You need a local RabbitMQ and MongoDB for those defaults to connect:

   ```sh
   docker run -d --name rabbitmq -p 5672:5672 -p 15672:15672 rabbitmq:3-management
   docker run -d --name mongo -p 27017:27017 mongo
   ```

   Instead of a local Mongo you can point `MONGO_URI` in `.env.local` (not
   `.env.example`) at a shared cluster - ask a maintainer for the connection
   string. If you do, set `MONGO_DB` to a name of your own (e.g.
   `proteng_<yourname>`), never `proteng-dev` / `proteng-production`, so your test
   data stays out of the shared databases.

   Done when: `.env.local` exists and RabbitMQ + a reachable MongoDB are up.

2. **Install dependencies**

   ```sh
   go mod tidy
   ```

   Done when: exits 0, no errors.

3. **Run** - `./run.sh` (Git Bash on Windows, or macOS/Linux terminal)

   (just sets `ENV=local` and runs `go run main.go` - `ENV` picks which `.env.<ENV>` file loads, there is no `.env.dev` anymore. Run manually with `ENV=local go run main.go` if you'd rather not use the script. Note: plain `cmd.exe`/PowerShell can't run `.sh` directly - use Git Bash.)

   Done when: terminal prints `proteng-conductor is running on :8081` (or whatever `HTTP_PORT` is set to), with no crash after.

## Format

`gofmt` autofixes on save/commit. Run manually against the whole repo:

```sh
gofmt -l -w .
```

## Lint

`go vet` reports issues but does not autofix - fix them by hand. Both this and `gofmt` also run in CI (`.github/workflows/test-build-dev.yaml`) on every push.

```sh
go vet ./...
```

## Pre-commit hooks

Format + lint above run automatically via [pre-commit](https://pre-commit.com/) on `git commit`; `go build` + `go test` additionally run on `git push`. See [CONTRIBUTING.md](./CONTRIBUTING.md) for details.

Install once per clone:

```sh
pip install pre-commit
pre-commit install --hook-type pre-commit --hook-type pre-push --hook-type commit-msg
```

Run everything manually: `pre-commit run --all-files`

## Generating mocks

Requires `mockgen`. Add a `go:generate` comment above the interface you want mocked (see examples in `repositories/`), then:

```sh
go generate ./...
```

## API docs

This service is called internally by proteng-bff only (frontend never calls it directly) - API docs live on **bff's** Swagger UI, not here: `http://localhost:8080/swagger/index.html` (see `proteng-bff/SETUP.md`).

## Build (optional, for deployment testing)

Env vars are not baked into the image - pass them at run time:

```sh
docker run -d --env-file .env.local proteng-conductor
```
