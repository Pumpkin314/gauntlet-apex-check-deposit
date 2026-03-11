FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN go install github.com/pressly/goose/v3/cmd/goose@latest
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api

FROM alpine:3.19
RUN apk add --no-cache curl postgresql-client
COPY --from=builder /go/bin/goose /usr/local/bin/goose
COPY --from=builder /api /api
COPY db/migrations /migrations
COPY db/seed.sql /seed.sql
EXPOSE 8080
COPY cmd/api/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
CMD ["/entrypoint.sh"]
