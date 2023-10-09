#build stage
FROM golang:1.20 AS builder
WORKDIR /go/src/proteng-conductor
ADD . .
RUN go get proteng-conductor
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags '-extldflags "-static"' -o app ./main.go

#final stage
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /go/src/proteng-conductor/app .
CMD ["./app"]

EXPOSE 8080