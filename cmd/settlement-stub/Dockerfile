FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY . .
RUN CGO_ENABLED=0 go build -o /settlement-stub ./cmd/settlement-stub

FROM alpine:3.19
RUN apk add --no-cache curl
COPY --from=builder /settlement-stub /settlement-stub
EXPOSE 8082
CMD ["/settlement-stub"]
