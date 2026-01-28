FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ingestor ./cmd/ingestor

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ingestor ./cmd/ingestor

# Copy entrypoint script
COPY docker-entrypoint.sh .
RUN chmod +x docker-entrypoint.sh

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl

WORKDIR /app

# Copy binary and entrypoint from builder
COPY --from=builder /app/ingestor .
COPY --from=builder /app/docker-entrypoint.sh .

# Environment variables with defaults
ENV BUCKET=blobsearch \
    PREFIX=logs \
    BATCH_SIZE=10000 \
    COMPRESSION=snappy \
    WITH_TIMESTAMPS=true \
    DEDUPLICATE=false \
    DEDUP_WINDOW=100000 \
    AUTO_FLUSH=true \
    AUTO_FLUSH_INTERVAL=90 \
    TIMESTAMP_FIELDS="timestamp,time,@timestamp" \
    LEVEL_FIELDS="level,severity,severityText" \
    HTTP_PORT=8080

# Expose ports (HTTP and GELF TCP)
EXPOSE 8080 12201

# Healthcheck
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/stats || exit 1

# Run the ingestor in HTTP mode via entrypoint
ENTRYPOINT ["./docker-entrypoint.sh"]
