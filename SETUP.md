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

3. **Run** — `ENV` picks which `.env.<ENV>` file loads (there is no `.env.dev` anymore):
   - macOS/Linux: `ENV=local go run main.go`
   - Windows CMD: `set ENV=local && go run main.go`
   - Windows PowerShell: `$Env:ENV = "local"; go run main.go`

   Done when: log shows the server listening on `HTTP_PORT` with no crash.

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

## Build (optional, for deployment testing)

Env vars are not baked into the image — pass them at run time:

```sh
docker run -d --env-file .env.local proteng-conductor
```
