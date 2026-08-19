#build stage
FROM golang:1.22 AS builder
WORKDIR /go/src/github.com/protengplus/proteng-conductor
ADD . .
RUN go get github.com/protengplus/proteng-conductor
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o app ./main.go

#final stage
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /go/src/github.com/protengplus/proteng-conductor/app .
CMD ["./app"]

EXPOSE 8080