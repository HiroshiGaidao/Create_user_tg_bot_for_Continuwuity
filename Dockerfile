FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o bot .

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/bot .
COPY messages/messages.yaml ./messages/

RUN mkdir -p /app/data /app/logs /app/matrix_store

CMD ["./bot"]