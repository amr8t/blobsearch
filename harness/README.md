# BlobSearch Development Harness

This directory contains development and testing infrastructure for BlobSearch.

**For production deployment, see the main [README.md](../README.md) and [QUICKSTART.md](../QUICKSTART.md).**

## What's Here

This harness provides:
- **MinIO** - Local S3-compatible storage for testing
- **BlobSearch Ingestor** - Built from source
- **Example Log Generators** - Apache and OTel log producers
- **Makefile** - Development commands

## Quick Start

From the project root:

```bash
# Start the dev harness
cd harness
make docker-up

# Generate and load test logs
make generate-logs type=apache count=10000 days=7
make load-logs file=../generated-apache.log

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
# Generate logs
make generate-logs type=apache count=1000 days=7
make generate-logs type=otel count=5000 days=30

# Load logs
make load-logs file=path/to/logs.log

# Flush and view stats
make flush
make stats
```

### Query

```bash
make query          # Start DuckDB with connection
make query-stats    # Quick statistics query
```

### Examples

```bash
# Run example log generators with forwarders
make example-up profile=apache
make example-up profile=webapp
make example-down
make example-logs profile=apache
```

## Directory Structure

```
harness/
├── docker-compose.yml   # MinIO + Ingestor + Examples
├── Makefile            # Dev commands
└── README.md           # This file
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
# Generate test logs
make generate-logs type=apache count=10000 days=7

# Load them
make load-logs file=../generated-apache.log

# Query results
make query-stats
```

### 5. Run Full Example

```bash
# Start example generator with forwarder
make example-up profile=apache

# Watch logs being forwarded
make example-logs profile=apache

# Query the ingested data
make query-stats

# Clean up
make example-down
```

## Testing Different Configurations

### High-Volume Ingestion

```bash
# Generate large dataset
make generate-logs type=apache count=100000 days=30

# Load it
make load-logs file=../generated-apache.log

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
make generate-logs type=otel count=50000 days=90

# Load and query specific date ranges
make load-logs file=../generated-otel.log
make flush

# Query in DuckDB
duckdb
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
ls -la ../examples/docker/generator/main.go

# Build it manually
cd ../examples/docker/generator
go build -o ../../../harness/generator .
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
- All-in-one docker-compose
- Development Makefile

**Production (main README):**
- Users provide their own S3 (AWS S3, MinIO, etc.)
- Pull pre-built images from ghcr.io
- Configure via environment variables
- Minimal setup

## Contributing

When developing new features:

1. Use this harness for testing
2. Generate test data with various configurations
3. Verify queries work correctly
4. Test the log forwarding
5. Update documentation
6. Run `make test` before committing

## See Also

- [Main README](../README.md) - Production deployment
- [QUICKSTART](../QUICKSTART.md) - End-user setup
- [QUERY_GUIDE](../QUERY_GUIDE.md) - Advanced querying
