# syntax=docker/dockerfile:1
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
COPY . .
RUN go build -o app main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/app .
COPY --from=builder /app/main.go .
COPY --from=builder /app/yt-dlp.exe .
ENV TELEGRAM_BOT_TOKEN=""
CMD ["/app/app"] 