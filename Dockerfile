FROM golang:1.22.4 AS builder
WORKDIR /app
COPY go.mod .
COPY go.sum .
RUN go mod download
# Кэшируем установку goose отдельно, чтобы не тянуть лишние зависимости при изменении исходников
RUN go install -tags 'postgres' github.com/pressly/goose/v3/cmd/goose@v3.22.0
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app main.go

FROM debian:bookworm
WORKDIR /app
RUN echo 'deb http://deb.debian.org/debian bookworm main contrib non-free non-free-firmware' > /etc/apt/sources.list && \
    apt-get update && \
    apt-get install -y ca-certificates git ffmpeg wget && \
    rm -rf /var/lib/apt/lists/*
COPY --from=builder /app/app .
COPY --from=builder /app/main.go .
COPY --from=builder /app/yt-dlp_linux .
COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/internal/i18n/translations ./internal/i18n/translations
COPY --from=builder /go/bin/goose /usr/local/bin/goose
RUN chmod +x /app/yt-dlp_linux
# Создаем папку tmp с правильными правами доступа
RUN mkdir -p /app/tmp && chmod 755 /app/tmp
ENV TELEGRAM_BOT_TOKEN=""
ENTRYPOINT sh -c "goose -dir /app/migrations postgres \"host=$DB_HOST user=$DB_USER password=$DB_PASSWORD dbname=$DB_NAME sslmode=disable\" up && /app/app" 