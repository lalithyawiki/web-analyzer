FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download
RUN go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags="-w -s" -o /analyzer-app ./cmd/main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /analyzer-app ./bin/analyzer-app
COPY --from=builder /app/ui ./ui

WORKDIR /app/bin

EXPOSE 8080

CMD ["./analyzer-app"]
