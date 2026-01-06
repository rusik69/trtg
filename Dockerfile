# Build stage
FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o trtg ./cmd/trtg

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/trtg .

RUN mkdir -p /app/downloads /app/data

VOLUME ["/app/downloads", "/app/data", "/app/torrents.txt"]

ENV TORRENTS_FILE=/app/torrents.txt
ENV DATABASE_PATH=/app/data/trtg.db
ENV DOWNLOAD_DIR=/app/downloads

ENTRYPOINT ["/app/trtg"]
