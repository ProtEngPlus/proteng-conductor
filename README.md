# proteng-conductor

## Developer Notes

required:

- go version 1.21+
- mockgen

Writing unit tests is a good practice for developers. We will mockgen to generate mock code to write unittests. Add `go:generate` comment to the file containing interfaces you want to mock. (examples in `repositories` folder)

And then run

```sh
go generate ./...
```

## Running in local

### 1. get `.env.dev` file from notion
### 2. install packages
```
go mod tidy
```
### 3. run development
- setting local environmental variable `ENV`, should be `dev`
- note that if you put set `ENV` to `<environment>` the app will load env vars from `.env.<environment>` file

MacOS
```
ENV=dev go run main.go
```
Windows - CMD
```
set ENV=dev
go run main.go
```
Windows - Powershell
```
$Env:ENV = "dev"
go run main.go
```

## Building
building with docker will not bring the env file to the image. Instead, you will have to specify in during the run time
```
docker run -d --env_file=".env.dev" proteng-conductor
```