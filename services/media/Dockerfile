FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/media ./cmd/media

# Distroless: no shell, no apt, ~2 MB base. Static binary only.
# Note: /media exists as an FHS directory in distroless — do not place the
# binary there or `exec` will see "is a directory".
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/media /usr/local/bin/media
EXPOSE 8002
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/media"]
