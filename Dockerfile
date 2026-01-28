FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a \
    -ldflags '-s -w -extldflags "-static"' \
    -o rls-http ./cmd/rls-http

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/rls-http .
COPY configs/rls.yaml /etc/pixiu-rls/config.yaml

EXPOSE 8080

ENTRYPOINT ["/app/rls-http"]
CMD ["-c", "/etc/pixiu-rls/config.yaml"]
