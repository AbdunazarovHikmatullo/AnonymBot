FROM golang:1.25.2-alpine AS builder

WORKDIR /app

# Кэш зависимостей
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходники
COPY . .

# Сборка бинарника
RUN go build -o bot main.go

# Этап выполнения
FROM alpine:latest

WORKDIR /app

RUN apk --no-cache add tzdata ca-certificates

COPY --from=builder /app/bot .

ENV TZ=Asia/Dushanbe \
    GIN_MODE=release

CMD ["./bot"]
