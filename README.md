# BlobSearch

High-performance log storage and analytics using Parquet compression and DuckDB. Built for modern web applications.

Why?
Cloud monitoring solutions while easy to use and lucrative only exist with free tiers to capture the market and the expectation is to squeeze profits with usage based pricing. Blobsearch is designed to be a cost-effective alternative to cloud-based solutions, providing a flexible and scalable platform for log storage and analytics.

## Comparison: BlobSearch vs Cloud-Based Log Solutions

| Feature | BlobSearch | Cloud-Based Solutions |
|---------|------------|----------------------|
| **Hosting** | Self-hosted | SaaS (CloudWatch, Datadog, LogDNA, Papertrail, etc.) |
| **Cost Model** | Storage + compute only | Per log ingestion + retention + host pricing |
| **Data Ownership** | Your S3 bucket | Vendor's servers |
| **Query Language** | SQL (DuckDB) | Proprietary query languages |
| **Data Format** | Parquet (open standard) | Vendor-specific formats |
| **Portability** | Full - export from S3 | Vendor lock-in |
| **Setup Time** | 5 minutes | 5-30 minutes |
| **Ingestion Rate** | 28K+ logs/sec | High (with throttling and tier limits) |
| **Compression** | 3.7x (Snappy) | ~2x |
| **Query Performance** | <50ms on 56K logs | Good on recent (??) |
| **Alerting** | DIY | Native |
| **Visualization** | DIY | Built-in dashboards |
| **Retention Cost** | S3 standard rates (predictable) | High for long retention (scales with usage) |
| **Open Source** | ✅ Yes | ❌ No |

### When to Choose BlobSearch

**Perfect for:**
- Startups wanting predictable costs
- Teams with SQL experience
- Multi-cloud or hybrid environments
- Projects requiring data portability
- Long-term log retention (months/years)
- Privacy-sensitive applications
- Pure log storage without analytics overhead

**Trade-offs:**
- No built-in UI (use BI tools like Grafana, Metabase)
- Alerting requires integration (e.g., CloudWatch Alarms on S3 metrics, Lambda functions)
- Self-managed infrastructure

## 🚀 Quickstart

### Run with Your S3

```bash
docker run -d -p 8080:8080 \
  -e ENDPOINT=https://s3.amazonaws.com \
  -e ACCESS_KEY=your-key \
  -e SECRET_KEY=your-secret \
  -e BUCKET=your-bucket \
  -e REGION=us-east-1 \
  ghcr.io/amr8t/blobsearch/ingestor:latest

# Send logs
echo '{"timestamp":"2024-01-15T10:30:00Z","level":"error","message":"Database connection failed"}' | \
  curl -X POST --data-binary @- http://localhost:8080/ingest

# Flush to S3
curl -X POST http://localhost:8080/flush
```

### Query with DuckDB

```sql
INSTALL httpfs; LOAD httpfs;
SET s3_region='us-east-1';
SET s3_access_key_id='your-key';
SET s3_secret_access_key='your-secret';

SELECT * FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true)
WHERE date = '2024-01-15' AND level = 'error'
LIMIT 10;
```

**For development/testing with MinIO:** See [harness/README.md](harness/README.md)

## Features

- **Fast** - 28K+ entries/sec ingestion
- **Efficient** - Parquet + Snappy (3.7x compression)
- **Quick Queries** - DuckDB queries in <50ms on 56K logs
- **S3-Compatible** - AWS S3, MinIO, DigitalOcean Spaces, etc.
- **Partitioned** - Hive-style partitioning by date/level
- **Docker Native** - Auto-collect logs from containers
- **Dedupe** - Optional deduplication
- **Auto-Flush** - Configurable automatic flushing (default: 90s)

## Architecture

```
┌─────────────┐
│   Your App  │ → HTTP POST / GELF / CLI
└──────┬──────┘
       │
       ↓
┌─────────────┐
│  Ingestor   │ → Parquet + Partition
└──────┬──────┘
       │ Parquet
       ↓
┌─────────────┐
│     S3      │
└──────┬──────┘
       │
       ↓
┌─────────────┐
│   DuckDB    │ → Query & Analyze
└─────────────┘
```

## Production Deployment

### Docker run example

Run ingestor on the host, containers send to host IP:

```bash
# Run ingestor on host
docker run -d \
  --name blobsearch-ingestor \
  --restart unless-stopped \
  -p 8080:8080 \
  -p 12201:12201 \
  -e ENDPOINT=https://s3.amazonaws.com \
  -e ACCESS_KEY=your-key \
  -e SECRET_KEY=your-secret \
  -e BUCKET=my-logs \
  -e REGION=us-east-1 \
  ghcr.io/amr8t/blobsearch/ingestor:latest

# Configure containers to use GELF
docker run -d \
  --log-driver=gelf \
  --log-opt gelf-address=tcp://172.17.0.1:12201 \
  --log-opt tag=my-app \
  your-app:latest
```

## Docker Compose Example

```yaml
version: '3.8'

services:
  ingestor:
    image: ghcr.io/amr8t/blobsearch/ingestor:latest
    ports:
      - "8080:8080"
      - "12201:12201"
    environment:
      ENDPOINT: https://s3.amazonaws.com
      ACCESS_KEY: ${AWS_ACCESS_KEY}
      SECRET_KEY: ${AWS_SECRET_KEY}
      BUCKET: my-logs
      REGION: us-east-1

  app:
    image: your-app:latest
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://ingestor:12201"
        tag: "my-app"
    depends_on:
      - ingestor
```

### Kubernetes

Deploy as a DaemonSet so every node runs an ingestor. Containers send logs to `localhost:12201`:

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: blobsearch-ingestor
spec:
  selector:
    matchLabels:
      app: blobsearch-ingestor
  template:
    metadata:
      labels:
        app: blobsearch-ingestor
    spec:
      hostNetwork: true
      containers:
      - name: ingestor
        image: ghcr.io/amr8t/blobsearch/ingestor:latest
        ports:
        - containerPort: 12201
          hostPort: 12201
        - containerPort: 8080
          hostPort: 8080
        env:
        - name: ENDPOINT
          value: "https://s3.amazonaws.com"
        - name: ACCESS_KEY
          valueFrom:
            secretKeyRef:
              name: s3-credentials
              key: access-key
        - name: SECRET_KEY
          valueFrom:
            secretKeyRef:
              name: s3-credentials
              key: secret-key
        - name: BUCKET
          value: "my-logs"
        - name: REGION
          value: "us-east-1"
```

Configure your pods to use GELF logging:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
spec:
  containers:
  - name: app
    image: your-app:latest
    # Docker GELF driver configuration
    # Add to docker daemon.json or use logging sidecar
```

**Note:** Configure Docker daemon on nodes with GELF driver pointing to `tcp://127.0.0.1:12201`


## Configuration

### Ingestor Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ENDPOINT` | *required* | S3 endpoint URL |
| `ACCESS_KEY` | *required* | S3 access key |
| `SECRET_KEY` | *required* | S3 secret key |
| `BUCKET` | `blobsearch` | S3 bucket name |
| `REGION` | `us-east-1` | S3 region |
| `PREFIX` | `logs` | S3 key prefix |
| `BATCH_SIZE` | `10000` | Logs per Parquet file |
| `COMPRESSION` | `snappy` | `snappy`, `gzip`, or `none` |
| `WITH_TIMESTAMPS` | `true` | Parse timestamps from logs |
| `DEDUPLICATE` | `false` | Enable deduplication |
| `DEDUP_WINDOW` | `100000` | Dedup cache size |
| `AUTO_FLUSH` | `true` | Enable automatic periodic flushing |
| `AUTO_FLUSH_INTERVAL` | `90` | Auto-flush interval in seconds |

## API

### POST /ingest
Ingest logs (newline-delimited text or JSON).

```bash
cat app.log | curl -X POST --data-binary @- http://localhost:8080/ingest
```

### POST /gelf
Ingest GELF formatted logs (HTTP endpoint).

```bash
# GELF messages can be sent via HTTP POST
curl -X POST -H "Content-Type: application/json" \
  --data '{"version":"1.1","host":"myhost","short_message":"Log message","timestamp":1234567890.123,"level":6}' \
  http://localhost:8080/gelf
```

### TCP Port 12201
Accept GELF messages via TCP (Docker GELF logging driver).

This is automatically enabled when running in HTTP mode. Configure your Docker containers:

```yaml
logging:
  driver: gelf
  options:
    gelf-address: "tcp://ingestor:12201"
    tag: "my-app"
```

**Note:** TCP is the default for reliability. For high-throughput scenarios where some message loss is acceptable, you can use UDP by starting a UDP server.

### POST /flush
Flush buffered logs to S3.

```bash
curl -X POST http://localhost:8080/flush
```

### GET /stats
Get ingestion statistics.

```bash
curl http://localhost:8080/stats
```

## Querying Logs

### Basic Queries

```sql
-- Count logs by level
SELECT level, COUNT(*) as count
FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true)
GROUP BY level;

-- Recent errors
SELECT timestamp, message
FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true)
WHERE level = 'error'
ORDER BY timestamp DESC
LIMIT 10;

-- Time series
SELECT date, COUNT(*) as log_count
FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true)
GROUP BY date
ORDER BY date DESC;
```

### Working with JSON Logs

```sql
-- Extract fields from JSON messages
SELECT 
    timestamp,
    level,
    json_extract_string(message, '$.service') as service,
    json_extract_string(message, '$.error_code') as error_code,
    message
FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true)
WHERE level = 'error'
LIMIT 10;

-- Count by service
SELECT 
    json_extract_string(message, '$.service') as service,
    COUNT(*) as count
FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true)
WHERE message LIKE '{%'
GROUP BY service
ORDER BY count DESC;
```

### Advanced Queries

See [QUERY_GUIDE.md](QUERY_GUIDE.md) for:
- Partition pruning for faster queries
- Deduplication strategies
- Performance optimization
- Complex aggregations

## Log Collection Methods

### GELF Logging Driver (Recommended)

Use Docker's native GELF logging driver for seamless integration:

```yaml
services:
  app:
    image: your-app:latest
    logging:
      driver: gelf
      options:
        gelf-address: "tcp://ingestor:12201"
        tag: "my-app"
```

**Advantages:**
- No additional containers
- No Docker socket access needed
- Native Docker integration
- TCP ensures reliable delivery
- Minimal overhead

See [examples/docker/README.md](examples/docker/README.md) for full examples.

## Performance

- **Ingestion**: 28,000+ logs/second
- **Compression**: Parquet with Snappy: 3.7x compression
- **Query**: <50ms for 56K logs
- **Partitioning**: 99.9% reduction in files scanned

## How It Works

### 1. Parquet Storage

Columnar format with compression:
- Fast analytical queries
- Excellent compression ratios (3-4x)
- Efficient for time-series data
- Native support in DuckDB

### 2. Hive Partitioning

Logs partitioned by: `date=YYYY-MM-DD/level=ERROR/`

**Benefits:**
- Query only relevant partitions
- 99.9% reduction in files scanned
- Sub-second queries on millions of logs
- Optimized for time-based and level-based filtering

### 3. Structured Logs

Optimized for JSON structured logs from modern frameworks:
- Next.js
- Rails
- Express
- FastAPI
- Any app outputting JSON logs

## Troubleshooting

### Logs not appearing in S3

```bash
# Check ingestor is running
curl http://localhost:8080/stats

# Flush manually
curl -X POST http://localhost:8080/flush

# Verify S3 credentials
aws s3 ls s3://your-bucket/logs/
```

### Can't query with DuckDB

```bash
# Test S3 connection
duckdb -c "
  INSTALL httpfs; LOAD httpfs;
  SET s3_region='us-east-1';
  SET s3_access_key_id='your-key';
  SET s3_secret_access_key='your-secret';
  SELECT * FROM read_parquet('s3://your-bucket/logs/date=*/level=*/*', hive_partitioning=true) LIMIT 1;
"
```

## Documentation

- **[QUERY_GUIDE.md](QUERY_GUIDE.md)** - Advanced querying and performance optimization
- **[harness/README.md](harness/README.md)** - Development/Testing environment

## Examples

- **[examples/standalone-docker/](examples/standalone-docker/)** - Simple Docker Compose with pre-built image
- **[examples/kubernetes/](examples/kubernetes/)** - Ultra-simple Kubernetes DaemonSet example

## Contributing

1. Fork the repository
2. Create a feature branch
3. Use the harness for testing: `cd harness && make docker-up`
4. Make changes and test
5. Run tests: `go test -v ./...`
6. Submit a pull request

## Support

- **Issues**: https://github.com/amr8t/blobsearch/issues
- **Discussions**: https://github.com/amr8t/blobsearch/discussions

## License

This project is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). See the [LICENSE](LICENSE) file for details.

This license requires that if you run a modified version of this software on a server and provide network access to users, you must make the modified source code available to those users.

## Credits

Built with:
- [Apache Parquet](https://parquet.apache.org/) for columnar storage
- [DuckDB](https://duckdb.org/) for analytics
- [MinIO](https://min.io/) for testing with S3-compatible storage
