# syntax=docker/dockerfile:1
FROM golang:1.24 AS builder
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN go install github.com/pressly/goose/v3/cmd/goose@latest
# Устанавливаем goose
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app main.go

FROM debian:bookworm-slim
WORKDIR /app
RUN apt-get update && apt-get install -y ca-certificates git && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/app .
COPY --from=builder /app/main.go .
COPY --from=builder /app/yt-dlp_linux .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /go/bin/goose /usr/local/bin/goose
RUN chmod +x /app/yt-dlp_linux
ENV TELEGRAM_BOT_TOKEN=""
CMD ["/app/app"] 