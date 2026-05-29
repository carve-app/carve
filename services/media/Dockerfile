FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod .
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /media ./cmd/media

FROM gcr.io/distroless/static-debian12
COPY --from=builder /media /media
EXPOSE 8002
ENTRYPOINT ["/media"]
