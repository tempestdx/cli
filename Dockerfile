FROM golang:1.23-alpine3.20 AS builder

WORKDIR /go/src/tempest
COPY . .

RUN go mod download && \
    go build -o /go/bin/tempest tempest/main.go

FROM alpine:3.21.0

USER nobody

COPY --from=builder /go/bin/tempest /usr/local/bin/tempest

ENTRYPOINT ["tempest"]
