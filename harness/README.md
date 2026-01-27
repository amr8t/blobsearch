# BlobSearch Development Harness

This directory contains development and testing infrastructure for BlobSearch.

**For production deployment, see the main [README.md](../README.md).**

## What's Here

This harness provides:
- **MinIO** - Local S3-compatible storage for testing
- **BlobSearch Ingestor** - Built from source
- **Log Generator** - Go-based structured log generator
- **Makefile** - Development commands

## Quick Start

```bash
# Start the dev harness
cd harness
make docker-up

# Generate and load test logs
make generate-logs count=10000 days=7
make load-logs file=../generated-logs.json

# Query
make query-stats
```

## Services

When you run `make docker-up`, the following services start:

| Service | Port | Description |
|---------|------|-------------|
| MinIO | 9000 | S3 API |
| MinIO Console | 9001 | Web UI (minioadmin/minioadmin) |
| Ingestor | 8080 | BlobSearch HTTP API |

## Makefile Commands

### Core Development

```bash
make build          # Build ingestor binary
make test           # Run tests
make clean          # Clean artifacts and volumes
```

### Docker Services

```bash
make docker-up      # Start all services (MinIO + Ingestor)
make docker-down    # Stop services
make docker-logs    # View service logs
```

### Log Generation & Loading

```bash
# Generate logs to file (OpenTelemetry format)
make generate-logs count=1000 days=7
make generate-logs count=5000 days=30

# Stream logs continuously (Docker container)
make stream-start       # Start streaming logs to ingestor
make stream-logs        # View generator output
make stream-stop        # Stop streaming

# Stream logs manually (without Docker)
make stream-manual delay=500ms batch=10

# Load logs from file
make load-logs file=path/to/logs.json

# Flush and view stats
make flush
make stats
```

### Query

```bash
make query          # Start DuckDB with connection
make query-stats    # Quick statistics query
```



## Directory Structure

```
harness/
├── docker-compose.yml      # MinIO + Ingestor
├── Dockerfile.generator    # Log generator image
├── generator/              # Go log generator source
│   └── main.go
├── Makefile               # Dev commands
└── README.md              # This file
```

## Development Workflow

### 1. Start the Harness

```bash
cd harness
make docker-up
```

### 2. Make Code Changes

Edit source files in `cmd/ingestor/` etc.

### 3. Rebuild and Test

```bash
# Rebuild ingestor
docker-compose build ingestor
docker-compose up -d ingestor

```

### 4. Test with Sample Data

```bash
# Generate test logs (OpenTelemetry format)
make generate-logs count=10000 days=7

# Load them
make load-logs file=../generated-logs.json

# Query results
make query-stats
```

### 5. Stream Logs in Real-Time

```bash
# Start log generator container (streams continuously)
make stream-start

# Monitor logs in another terminal
make stream-logs

# Watch stats in another terminal
watch -n 1 'curl -s http://localhost:8080/stats | jq'

# Stop streaming
make stream-stop

# Query the accumulated data
make query-stats
```

## Testing Different Configurations

### High-Volume Ingestion

```bash
# Generate large dataset
make generate-logs count=100000 days=30

# Load it
make load-logs file=../generated-logs.json

# Monitor performance
make stats
```

### Deduplication Testing

Edit `docker-compose.yml` to enable deduplication:

```yaml
ingestor:
  environment:
    DEDUPLICATE: "true"
    DEDUP_WINDOW: "100000"
```

Restart:
```bash
make docker-down
make docker-up
```

### Compression Testing

Test different compression methods:

```yaml
ingestor:
  environment:
    COMPRESSION: "gzip"  # or "snappy" or "none"
```

### Date Range Partitioning

```bash
# Generate logs across multiple months
make generate-logs count=50000 days=90

# Load and query specific date ranges
make load-logs file=../generated-logs.json
make flush

# Query in DuckDB with date filtering
make query
```

```sql
SELECT date, COUNT(*) 
FROM read_parquet('s3://blobsearch/logs/**/*', hive_partitioning=true)
WHERE date >= '2024-01-01' AND date <= '2024-01-31'
GROUP BY date
ORDER BY date;
```

## Cleaning Up

```bash
# Stop services and remove volumes
make clean

# Or just stop without removing data
make docker-down
```

## MinIO Console

Access the MinIO web interface at http://localhost:9001

**Credentials:**
- Username: `minioadmin`
- Password: `minioadmin`

You can:
- Browse the `blobsearch` bucket
- View Parquet files
- Monitor storage usage
- Create additional buckets

## Troubleshooting

### Services Won't Start

```bash
# Check logs
make docker-logs

# Verify ports aren't in use
lsof -i :8080
lsof -i :9000
lsof -i :9001

# Clean and restart
make clean
make docker-up
```

### Generator Not Found

```bash
# Verify generator exists
ls -la generator/main.go

# Test it directly
cd generator && go run main.go -count 10 -output test.json

# Test streaming to ingestor
cd generator && go run main.go -stream -delay 1s -endpoint http://localhost:8080/ingest -batch 10
```

### Ingestor Not Responding

```bash
# Check if running
curl http://localhost:8080/stats

# Rebuild
docker-compose build ingestor
docker-compose up -d ingestor

# Check logs
docker-compose logs ingestor
```

### Can't Query Logs

```bash
# Ensure logs are flushed
make flush

# Verify files exist in MinIO
# Go to http://localhost:9001, browse blobsearch/logs/

# Test DuckDB connection
duckdb -c "
  INSTALL httpfs; LOAD httpfs;
  SET s3_endpoint='localhost:9000';
  SET s3_access_key_id='blobsearch';
  SET s3_secret_access_key='blobsearch123';
  SET s3_use_ssl=false;
  SELECT 1 as test;
"
```

## Production vs Harness

**Harness (this directory):**
- For development and testing only
- Includes MinIO for local S3 storage
- Builds ingestor from source
- Includes log generator
- Development Makefile

**Production (see examples/):**
- **standalone-docker/** - Docker Compose with pre-built images
- **kubernetes/** - Kubernetes DaemonSet pattern
- Uses real S3 or MinIO
- Minimal setup

## Contributing

When developing new features:

1. Use this harness for testing
2. Generate test data with various configurations
3. Verify queries work correctly
4. Test the log forwarding
5. Update documentation
6. Run `make test` before committing

## Generator Usage

The log generator supports multiple modes:

```bash
# Generate to file
cd generator
go run main.go -count 1000 -output logs.json

# Generate specific date range
go run main.go -count 5000 -days 30 -start-date 2024-01-01 -output logs.json

# Stream to stdout
go run main.go -stream -delay 500ms

# Stream directly to HTTP endpoint
go run main.go -stream -delay 1s -endpoint http://localhost:8080/ingest -batch 10

# Batch POST to endpoint
go run main.go -count 10000 -endpoint http://localhost:8080/ingest -batch 100
```

**Docker Mode:**
```bash
# Start generator container (streams to ingestor)
make stream-start

# View logs
make stream-logs

# Stop generator
make stream-stop
```

## See Also

- [Main README](../README.md) - Overview and features
- [examples/standalone-docker/](../examples/standalone-docker/) - Simple Docker setup
- [examples/kubernetes/](../examples/kubernetes/) - Kubernetes deployment
- [QUERY_GUIDE.md](../QUERY_GUIDE.md) - Advanced querying
