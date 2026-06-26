FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /api ./cmd/api \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /migrate ./cmd/migrate

FROM alpine:3.21
# hadolint ignore=DL3018
RUN apk add --no-cache ca-certificates tzdata wget \
 && addgroup -S app && adduser -S -G app app

WORKDIR /app
COPY --from=builder --chown=app:app /api /migrate ./
COPY --chown=app:app migrations/ ./migrations/

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/health/ready || exit 1

CMD ["./api"]
