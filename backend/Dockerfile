# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/api ./cmd/api

# Run stage
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/bin/api ./api
COPY --from=builder /app/migrations ./migrations

# Default port
EXPOSE 8080

CMD ["./api"]
