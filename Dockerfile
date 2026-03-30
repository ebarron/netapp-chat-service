FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /chat-service ./cmd/chat-service

FROM gcr.io/distroless/static-debian12

COPY --from=builder /chat-service /chat-service

EXPOSE 8090

ENTRYPOINT ["/chat-service"]
CMD ["-config", "/etc/chat-service/config.yaml"]
