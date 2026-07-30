# Setup

## Run locally

1. **Copy the env file**
   ```sh
   cp .env.example .env.local
   ```
   Fill in real values. Done when: `.env.local` exists with real values (not the empty template).

2. **Install dependencies**
   ```sh
   go mod tidy
   ```
   Done when: exits 0, no errors.

3. **Run** — `./run.sh` (Git Bash on Windows, or macOS/Linux terminal)

   (just sets `ENV=local` and runs `go run main.go` — `ENV` picks which `.env.<ENV>` file loads, there is no `.env.dev` anymore. Run manually with `ENV=local go run main.go` if you'd rather not use the script. Note: plain `cmd.exe`/PowerShell can't run `.sh` directly — use Git Bash.)

   Done when: terminal prints `proteng-conductor is running on :8081` (or whatever `HTTP_PORT` is set to), with no crash after.

## Format

`gofmt` autofixes on save/commit. Run manually against the whole repo:

```sh
gofmt -l -w .
```

## Lint

`go vet` reports issues but does not autofix — fix them by hand:

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

This service is called internally by proteng-bff only (frontend never calls it directly) — API docs live on **bff's** Swagger UI, not here: `http://localhost:8080/swagger/index.html` (see `proteng-bff/SETUP.md`).

## Build (optional, for deployment testing)

Env vars are not baked into the image — pass them at run time:

```sh
docker run -d --env-file .env.local proteng-conductor
```
