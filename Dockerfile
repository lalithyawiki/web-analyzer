
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -ldflags="-w -s" -o /analyzer-app ./main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /analyzer-app .
COPY --from=builder /app/templates ./templates

EXPOSE 8080

CMD ["./analyzer-app"]
