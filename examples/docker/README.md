# BlobSearch Docker Examples

Example application that demonstrates log collection using Docker's native GELF logging driver to forward structured JSON logs directly to BlobSearch.

## Structure

```
examples/docker/
├── generator/          # Go-based JSON log generator
│   └── main.go
├── app.py             # Python JSON log generator
├── Dockerfile         # Dockerfile for app.py
├── Dockerfile.generator # Dockerfile for Go generator
└── docker-compose.yml # Example service with GELF logging
```

## Quick Start

### 1. Start BlobSearch (from project root)

```bash
cd ../..
make docker-up
```

This starts MinIO and the ingestor service.

### 2. Run Example Log Generator

```bash
make example-up
```

This starts:
- `blobsearch-ingestor` - Receives GELF logs on UDP port 12201
- `webapp-log-app` - Generates structured JSON logs sent via GELF

### 3. Monitor Ingestion

```bash
# View ingestor stats
make stats

# Flush to storage
make flush

# Query logs
make query-stats
```

## What's Included

### Structured JSON Logs

The Python app generates realistic structured JSON logs:

```json
{
  "timestamp": "2024-01-15T10:30:45Z",
  "level": "error",
  "service": "auth-service",
  "message": "Database connection failed: connection timeout",
  "request_id": "req_abc123def456",
  "user_id": "user_1234",
  "endpoint": "/api/v1/users",
  "method": "POST",
  "duration_ms": 3500,
  "status_code": 500,
  "error_code": "ERR_DB_CONNECTION",
  "trace_id": "abc123def456789",
  "span_id": "def456789012"
}
```

### GELF Logging Driver

Instead of a separate log forwarder container, we use Docker's native GELF logging driver:

**Benefits:**
- ✅ No sidecar container needed
- ✅ No Docker socket mount required (more secure)
- ✅ Native Docker integration
- ✅ Lower resource usage
- ✅ Built-in buffering and retry logic

**Configuration:**
```yaml
webapp-log-app:
  logging:
    driver: gelf
    options:
      gelf-address: "tcp://blobsearch-ingestor:12201"
      tag: "webapp"
```

## Architecture

```
┌─────────────────┐
│   Python App    │
│  JSON logs      │
│   → stdout      │
└────────┬────────┘
         │
         │ Docker GELF Driver
         │ (TCP port 12201)
         ↓
┌─────────────────┐
│   Ingestor      │
│  - GELF → JSON  │
│  - Parquet      │
│  - Partition    │
└────────┬────────┘
         │ Parquet files
         ↓
┌─────────────────┐
│   MinIO/S3      │
└────────┬────────┘
         │
         ↓
┌─────────────────┐
│     DuckDB      │
│  Query & Analyze│
└─────────────────┘
```

## Using GELF in Your App

### Basic Configuration

Add the GELF logging driver to any service in your `docker-compose.yml`:

```yaml
services:
  your-app:
    image: your-app:latest
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://blobsearch-ingestor:12201"
        tag: "your-app"
    networks:
      - blobsearch
    depends_on:
      - blobsearch-ingestor
```

### Advanced GELF Options

```yaml
logging:
  driver: gelf
  options:
    gelf-address: "tcp://blobsearch-ingestor:12201"
    tag: "{{.Name}}"                    # Use container name as tag
    gelf-compression-type: "gzip"       # Enable compression
    gelf-compression-level: "6"         # Compression level (1-9)
```

### UDP GELF (Alternative)

For ultra-high throughput scenarios where some message loss is acceptable, you can use UDP:

```yaml
logging:
  driver: gelf
  options:
    gelf-address: "udp://blobsearch-ingestor:12201"
    tag: "your-app"
```

**Note:** TCP (default) is more reliable. UDP is faster but may lose messages under high load.

## GELF Message Format

Docker's GELF driver automatically converts your logs to GELF format:

```json
{
  "version": "1.1",
  "host": "container-hostname",
  "short_message": "Your log message",
  "timestamp": 1642248645.123,
  "level": 6,
  "facility": "docker",
  "_container_name": "webapp-log-app",
  "_image_name": "webapp:latest",
  "_tag": "webapp"
}
```

The ingestor converts GELF to BlobSearch's internal format:
- Extracts `short_message` → `message`
- Converts syslog `level` (0-7) → standard levels (error/warn/info/debug)
- Preserves all extra fields (those starting with `_`)

## Using the Generator

### Generate Logs Locally

```bash
# From project root
make generate-logs count=10000 days=7

# Or from generator directory
cd generator
go run main.go -count 10000 -days 7 -output logs.json
```

### Generator Options

```bash
go run main.go [options]

Options:
  -count int
        Number of log entries to generate (default 1000)
  -days int
        Number of days to span logs across (default 1)
  -start-date string
        Start date for log timestamps (format: 2006-01-02, default: today)
  -output string
        Output file path (writes to stdout if not specified)
  -stream
        Stream mode: continuously generate logs (Ctrl+C to stop)
  -delay duration
        Delay between logs in stream mode (default 1s)
```

### Examples

```bash
# Generate 5000 logs to file
go run main.go -count 5000 -output logs.json

# Generate logs for past 30 days
go run main.go -count 50000 -days 30 -output logs.json

# Stream logs continuously
go run main.go -stream -delay 500ms

# Pipe directly to ingestor (bypassing GELF)
go run main.go -count 10000 | curl -X POST --data-binary @- http://localhost:8080/ingest
```

## Querying Examples

Once logs are ingested and flushed, query them with DuckDB:

```sql
-- Recent logs
SELECT timestamp, message, level, host 
FROM logs 
ORDER BY timestamp DESC 
LIMIT 10;

-- Count by service
SELECT 
    json_extract_string(message, '$.service') as service,
    COUNT(*) as count
FROM logs
GROUP BY service
ORDER BY count DESC;

-- Error analysis
SELECT 
    json_extract_string(message, '$.error_code') as error_code,
    COUNT(*) as count
FROM logs
WHERE level = 'error'
GROUP BY error_code
ORDER BY count DESC;

-- Distributed tracing
SELECT 
    timestamp,
    host,
    json_extract_string(message, '$.service') as service,
    json_extract_string(message, '$.message') as msg
FROM logs
WHERE json_extract_string(message, '$.trace_id') = 'abc123def456'
ORDER BY timestamp;
```

## Development Workflow

### 1. Start Services

```bash
cd ../../harness
make docker-up
cd ../examples/docker
docker-compose up -d
```

### 2. Watch Logs Being Generated

```bash
docker logs -f webapp-log-app
```

### 3. Check Ingestion

```bash
curl http://localhost:8080/stats | jq
```

### 4. Flush to Storage

```bash
curl -X POST http://localhost:8080/flush
```

### 5. Query Results

```bash
cd ../../harness
make query-stats
```

## Customizing for Your App

### Any Language, Any Framework

The beauty of GELF is that it works with any application:

```yaml
services:
  nextjs-app:
    image: my-nextjs-app
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://blobsearch-ingestor:12201"
        tag: "nextjs"

  rails-app:
    image: my-rails-app
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://blobsearch-ingestor:12201"
        tag: "rails"

  python-api:
    image: my-fastapi-app
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://blobsearch-ingestor:12201"
        tag: "api"
```

### Recommended JSON Log Format

For best results, output structured JSON logs:

```json
{
  "timestamp": "2024-01-15T10:30:45Z",  // ISO 8601 format
  "level": "error",                      // error, warn, info, debug
  "service": "api-service",              // service name
  "message": "Error message here",       // log message
  "request_id": "req_abc123",            // optional: request tracking
  "user_id": "user_123",                 // optional: user context
  "error_code": "ERR_DB_CONNECTION",     // optional: error categorization
  "duration_ms": 150                     // optional: timing data
}
```

## Performance Notes

- **Throughput**: GELF TCP can handle 50,000+ messages/second
- **Latency**: Near real-time (sub-second)
- **Memory**: Minimal overhead (native Docker driver)
- **Compression**: Parquet with Snappy provides 3.7x compression
- **Reliability**: TCP ensures all messages are delivered (recommended for production)

## Troubleshooting

### Logs Not Appearing in BlobSearch

```bash
# Check if GELF TCP port is listening
docker exec blobsearch-ingestor netstat -tln | grep 12201

# Check ingestor logs for GELF errors
docker logs blobsearch-ingestor

# Verify network connectivity (TCP)
docker exec webapp-log-app nc -zv blobsearch-ingestor 12201

# Check ingestor stats
curl http://localhost:8080/stats
```

### Container Can't Resolve blobsearch-ingestor

```bash
# Verify both containers are on the same network
docker network inspect harness_blobsearch

# Check DNS resolution
docker exec webapp-log-app nslookup blobsearch-ingestor
```

### GELF Driver Not Available

```bash
# Verify Docker version (GELF driver available in Docker 1.12+)
docker version

# Check available log drivers
docker info | grep "Logging Driver"
```

### Connection Issues

If containers can't connect to the GELF endpoint:

1. **Verify network**: Ensure all containers are on the same Docker network
2. **Check service name**: Use the correct container/service name in gelf-address
3. **Check port**: Ensure port 12201 is not blocked
4. **Try UDP**: If TCP has issues, UDP (`udp://host:12201`) may work as fallback

## Cleanup

```bash
# Stop examples
make example-down

# Stop all services
cd ../../harness
make docker-down
```

## Next Steps

- See [../../harness/README.md](../../harness/README.md) for development environment
- See [../../QUERY_GUIDE.md](../../QUERY_GUIDE.md) for advanced queries
- See [../../README.md](../../README.md) for full BlobSearch documentation