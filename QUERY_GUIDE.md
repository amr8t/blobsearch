# BlobSearch Query Guide

Complete guide for querying logs with DuckDB. Optimized for structured JSON logs from modern web frameworks (Next.js, Rails, Express, FastAPI, etc.).

## Table of Contents
1. [Setup](#setup)
2. [Ingestion via HTTP API](#ingestion-via-http-api)
3. [Basic Queries](#basic-queries)
4. [JSON Log Queries](#json-log-queries)
5. [Time-Based Analysis](#time-based-analysis)
6. [Performance Optimization](#performance-optimization)
7. [Deduplication](#deduplication)
8. [Export & Reporting](#export--reporting)
9. [Troubleshooting](#troubleshooting)

---

## Setup

### Initial Configuration

```sql
-- Install and load S3 support
INSTALL httpfs;
LOAD httpfs;

-- Configure S3 connection (MinIO example)
SET s3_endpoint='localhost:9000';
SET s3_access_key_id='blobsearch';
SET s3_secret_access_key='blobsearch123';
SET s3_region='us-east-1';
SET s3_use_ssl=false;
SET s3_url_style='path';

-- For AWS S3
SET s3_region='us-east-1';
SET s3_access_key_id='your-key';
SET s3_secret_access_key='your-secret';
-- No need to set endpoint for AWS S3
```

### Create View

**IMPORTANT**: Use `/**/*` pattern for partitioned files

```sql
-- ✅ CORRECT - Matches all parquet files with specific partition pattern
CREATE OR REPLACE VIEW logs AS
SELECT * FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*', hive_partitioning = true);

-- Or query directly without a view
SELECT * FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*', hive_partitioning = true) LIMIT 10;
```

---

## Ingestion via HTTP API

### Quick Start

```bash
# Start ingestor (see README.md for full setup)
docker run -d -p 8080:8080 \
  -e ENDPOINT=http://minio:9000 \
  -e ACCESS_KEY=blobsearch \
  -e SECRET_KEY=blobsearch123 \
  -e BUCKET=blobsearch \
  ghcr.io/amr8t/blobsearch/ingestor:latest

# Ingest JSON logs
echo '{"timestamp":"2024-01-15T10:30:00Z","level":"error","service":"api","message":"Connection timeout"}' | \
  curl -X POST --data-binary @- http://localhost:8080/ingest

# Ingest text logs
echo "2024-01-15 [ERROR] Database connection failed" | \
  curl -X POST --data-binary @- http://localhost:8080/ingest

# Flush to S3
curl -X POST http://localhost:8080/flush
```

### API Endpoints

**POST /ingest** - Ingest logs (newline-delimited)
```bash
cat app.log | curl -X POST --data-binary @- http://localhost:8080/ingest

# Response:
{
  "status": "ok",
  "lines_processed": 1000,
  "total_lines": 1000,
  "partitions": 5,
  "unique_lines": 1000
}
```

**POST /flush** - Force write to S3
```bash
curl -X POST http://localhost:8080/flush

# Response:
{
  "status": "flushed",
  "total_lines": 1000,
  "unique_lines": 1000,
  "partitions": 5
}
```

**GET /stats** - Get statistics
```bash
curl http://localhost:8080/stats

# Response:
{
  "total_lines": 1000,
  "unique_lines": 1000,
  "partitions": 5,
  "dedup_enabled": false
}
```

---

## Basic Queries

### Dataset Overview

```sql
-- Count total logs
SELECT COUNT(*) as total_logs FROM logs;

-- See date range
SELECT 
    MIN(timestamp) as first_log,
    MAX(timestamp) as last_log,
    COUNT(*) as total
FROM logs;

-- Sample 10 recent logs
SELECT timestamp, level, message 
FROM logs 
ORDER BY timestamp DESC 
LIMIT 10;
```

### Log Level Distribution

```sql
-- Count by level
SELECT 
    level,
    COUNT(*) as count,
    ROUND(100.0 * COUNT(*) / (SELECT COUNT(*) FROM logs), 2) as percentage
FROM logs
GROUP BY level
ORDER BY count DESC;

-- Expected output:
-- level   | count  | percentage
-- --------|--------|------------
-- info    | 45000  | 75.00
-- error   | 10000  | 16.67
-- warn    | 5000   | 8.33
```

### Date Distribution

```sql
-- Logs per day
SELECT 
    date,
    COUNT(*) as log_count,
    COUNT(DISTINCT level) as distinct_levels
FROM logs
GROUP BY date
ORDER BY date DESC;

-- Date range query
SELECT date, COUNT(*) as count
FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*', hive_partitioning=true)
WHERE date >= '2026-01-01' AND date <= '2026-01-30'
GROUP BY date
ORDER BY date;
```

### Search for Specific Content

```sql
-- Full-text search in messages
SELECT timestamp, level, message
FROM logs
WHERE message LIKE '%timeout%'
ORDER BY timestamp DESC
LIMIT 20;

-- Case-insensitive search
SELECT timestamp, level, message
FROM logs
WHERE LOWER(message) LIKE LOWER('%Connection%')
ORDER BY timestamp DESC
LIMIT 20;

-- Multiple conditions
SELECT timestamp, level, message
FROM logs
WHERE level = 'error'
  AND message LIKE '%database%'
ORDER BY timestamp DESC;
```

---

## JSON Log Queries

Modern frameworks output JSON structured logs. DuckDB provides powerful JSON functions to query these.

### Extract JSON Fields

```sql
-- Extract common fields from JSON logs
SELECT 
    timestamp,
    level,
    json_extract_string(message, '$.service') as service,
    json_extract_string(message, '$.error_code') as error_code,
    json_extract_string(message, '$.message') as log_message
FROM logs
WHERE message LIKE '{%'  -- Only JSON logs
LIMIT 10;
```

### Service-Based Analysis

```sql
-- Count by service
SELECT 
    json_extract_string(message, '$.service') as service,
    COUNT(*) as total_logs,
    COUNT(*) FILTER (WHERE level = 'error') as errors,
    COUNT(*) FILTER (WHERE level = 'warn') as warnings
FROM logs
WHERE message LIKE '{%'
GROUP BY service
ORDER BY total_logs DESC;
```

### Error Analysis

```sql
-- Group errors by error code
SELECT 
    json_extract_string(message, '$.error_code') as error_code,
    COUNT(*) as count,
    json_extract_string(ANY_VALUE(message), '$.message') as example_message
FROM logs
WHERE level = 'error'
  AND message LIKE '%error_code%'
GROUP BY error_code
ORDER BY count DESC;
```

### Request Tracking

```sql
-- Find all logs for a specific request
SELECT 
    timestamp,
    level,
    json_extract_string(message, '$.service') as service,
    json_extract_string(message, '$.message') as log_message,
    message
FROM logs
WHERE message LIKE '%"request_id":"abc123"%'
ORDER BY timestamp;

-- Track request across services (distributed tracing)
SELECT 
    timestamp,
    json_extract_string(message, '$.service') as service,
    json_extract_string(message, '$.trace_id') as trace_id,
    json_extract_string(message, '$.endpoint') as endpoint,
    json_extract_string(message, '$.duration_ms') as duration_ms
FROM logs
WHERE message LIKE '%"trace_id":"95e2f0b24f0ddeaa"%'
ORDER BY timestamp;
```

### Performance Monitoring

```sql
-- Slow endpoints (duration > 3000ms)
SELECT 
    date,
    json_extract_string(message, '$.endpoint') as endpoint,
    AVG(CAST(json_extract_string(message, '$.duration_ms') AS INTEGER)) as avg_duration_ms,
    COUNT(*) as slow_requests
FROM logs
WHERE message LIKE '%"duration_ms"%'
  AND CAST(json_extract_string(message, '$.duration_ms') AS INTEGER) > 3000
GROUP BY date, endpoint
ORDER BY date DESC, avg_duration_ms DESC;

-- P95 latency by endpoint
SELECT 
    json_extract_string(message, '$.endpoint') as endpoint,
    APPROX_QUANTILE(CAST(json_extract_string(message, '$.duration_ms') AS INTEGER), 0.95) as p95_ms,
    AVG(CAST(json_extract_string(message, '$.duration_ms') AS INTEGER)) as avg_ms,
    COUNT(*) as requests
FROM logs
WHERE message LIKE '%"duration_ms"%'
GROUP BY endpoint
ORDER BY p95_ms DESC;
```

### User Activity

```sql
-- Logs by user
SELECT 
    json_extract_string(message, '$.user_id') as user_id,
    COUNT(*) as action_count,
    COUNT(*) FILTER (WHERE level = 'error') as errors,
    MIN(timestamp) as first_seen,
    MAX(timestamp) as last_seen
FROM logs
WHERE message LIKE '%"user_id"%'
GROUP BY user_id
ORDER BY action_count DESC
LIMIT 20;
```

---

## Time-Based Analysis

### Hourly Breakdown

```sql
-- Logs per hour
SELECT 
    DATE_TRUNC('hour', timestamp) as hour,
    COUNT(*) as log_count,
    COUNT(*) FILTER (WHERE level = 'error') as errors
FROM logs
GROUP BY hour
ORDER BY hour DESC;
```

### Daily Error Trends

```sql
-- Error rate by day
SELECT 
    date,
    COUNT(*) as total_logs,
    COUNT(*) FILTER (WHERE level = 'error') as errors,
    ROUND(100.0 * COUNT(*) FILTER (WHERE level = 'error') / COUNT(*), 2) as error_rate_pct
FROM logs
GROUP BY date
ORDER BY date DESC;
```

### Week-Over-Week Comparison

```sql
-- Compare this week vs last week
WITH weekly_stats AS (
    SELECT 
        DATE_TRUNC('week', timestamp) as week,
        COUNT(*) as total_logs,
        COUNT(*) FILTER (WHERE level = 'error') as errors
    FROM logs
    GROUP BY week
)
SELECT 
    week,
    total_logs,
    errors,
    LAG(total_logs) OVER (ORDER BY week) as prev_week_logs,
    LAG(errors) OVER (ORDER BY week) as prev_week_errors,
    ROUND(100.0 * (total_logs - LAG(total_logs) OVER (ORDER BY week)) / 
          NULLIF(LAG(total_logs) OVER (ORDER BY week), 0), 2) as log_growth_pct
FROM weekly_stats
ORDER BY week DESC;
```

### Peak Activity Detection

```sql
-- Find peak activity hours
SELECT 
    EXTRACT(hour FROM timestamp) as hour_of_day,
    EXTRACT(dow FROM timestamp) as day_of_week,
    COUNT(*) as log_count
FROM logs
GROUP BY hour_of_day, day_of_week
ORDER BY log_count DESC
LIMIT 10;
```

### Monthly Trends

```sql
-- Monthly summary
SELECT 
    DATE_TRUNC('month', timestamp) as month,
    COUNT(*) as total_logs,
    COUNT(DISTINCT date) as active_days,
    COUNT(*) / COUNT(DISTINCT date) as avg_logs_per_day,
    COUNT(*) FILTER (WHERE level = 'error') as errors,
    COUNT(*) FILTER (WHERE level = 'warn') as warnings
FROM logs
GROUP BY month
ORDER BY month DESC;
```

---

## Performance Optimization

### Partition Pruning

BlobSearch partitions logs by `date` and `level`. Use these fields in WHERE clauses for maximum performance.

#### Partitioned Query Examples

```sql
-- ✅ FAST - Uses partition pruning (scans only relevant files)
SELECT COUNT(*) 
FROM logs 
WHERE date = '2024-01-15' AND level = 'error';

-- ✅ FAST - Date range with level
SELECT timestamp, message
FROM logs
WHERE date BETWEEN '2024-01-10' AND '2024-01-15'
  AND level = 'error'
ORDER BY timestamp DESC;

-- ❌ SLOWER - No partition filters (scans all files)
SELECT COUNT(*) 
FROM logs 
WHERE message LIKE '%timeout%';

-- ✅ OPTIMIZED - Combine partition filters with other conditions
SELECT COUNT(*) 
FROM logs 
WHERE date = '2024-01-15'
  AND level = 'error'
  AND message LIKE '%timeout%';
```

### Query Profiling

```sql
-- Enable profiling
PRAGMA enable_profiling;

-- Run your query
SELECT date, level, COUNT(*) 
FROM logs 
WHERE date >= '2024-01-01'
GROUP BY date, level;

-- View profile
PRAGMA show_profile;
```

### Performance Tips

1. **Always filter by date when possible**
   ```sql
   -- Good
   WHERE date = '2024-01-15'
   
   -- Also good
   WHERE date BETWEEN '2024-01-10' AND '2024-01-15'
   ```

2. **Add level filters for error/warn queries**
   ```sql
   -- Good
   WHERE date = '2024-01-15' AND level = 'error'
   ```

3. **Use LIMIT for exploration**
   ```sql
   -- Faster for initial exploration
   SELECT * FROM logs LIMIT 100;
   ```

4. **Avoid SELECT * for large datasets**
   ```sql
   -- Better
   SELECT timestamp, level, message FROM logs;
   ```

5. **Use aggregations instead of fetching all rows**
   ```sql
   -- Much faster than fetching millions of rows
   SELECT date, COUNT(*) FROM logs GROUP BY date;
   ```

### Indexing

Parquet files have built-in column statistics that act as indexes:
- Min/max values per column per row group
- Null counts
- Automatically used for pruning

No manual indexing needed!

---

## Deduplication

### Understanding Deduplication

BlobSearch can deduplicate logs at ingestion time:

```bash
# Enable deduplication
docker run -d -p 8080:8080 \
  -e DEDUPLICATE=true \
  -e DEDUP_WINDOW=100000 \
  -e ... \
  ghcr.io/amr8t/blobsearch/ingestor:latest
```

**How it works:**
- Computes hash of `message + timestamp`
- Keeps sliding window of recent hashes (default: 100k)
- Skips duplicate entries within the window

### Query-Time Deduplication

If logs contain duplicates, deduplicate during queries:

```sql
-- Deduplicate by message + timestamp
SELECT DISTINCT ON (message, timestamp)
    timestamp, level, message
FROM logs
WHERE date = '2024-01-15'
ORDER BY message, timestamp, line_number;

-- Count unique messages per day
SELECT 
    date,
    COUNT(DISTINCT message) as unique_messages,
    COUNT(*) as total_logs
FROM logs
GROUP BY date;
```

### Finding Duplicates

```sql
-- Find duplicate log messages
SELECT 
    message,
    COUNT(*) as occurrence_count,
    MIN(timestamp) as first_seen,
    MAX(timestamp) as last_seen
FROM logs
WHERE date = '2024-01-15'
GROUP BY message
HAVING COUNT(*) > 1
ORDER BY occurrence_count DESC;
```

---

## Export & Reporting

### Export to CSV

```sql
-- Daily error summary
COPY (
    SELECT 
        date,
        COUNT(*) as error_count,
        COUNT(DISTINCT message) as unique_errors
    FROM logs
    WHERE level = 'error'
    GROUP BY date
    ORDER BY date DESC
) TO 'daily_errors.csv' WITH (HEADER, DELIMITER ',');
```

### Export to JSON

```sql
-- Export recent errors as JSON
COPY (
    SELECT 
        timestamp,
        level,
        message
    FROM logs
    WHERE level = 'error'
      AND date = '2024-01-15'
    ORDER BY timestamp DESC
    LIMIT 100
) TO 'recent_errors.json';
```

### Error Report

```sql
-- Comprehensive error report
COPY (
    SELECT 
        date,
        level,
        json_extract_string(message, '$.service') as service,
        json_extract_string(message, '$.error_code') as error_code,
        COUNT(*) as count,
        ANY_VALUE(message) as example
    FROM logs
    WHERE level IN ('error', 'warn')
      AND date >= '2024-01-01'
    GROUP BY date, level, service, error_code
    ORDER BY date DESC, count DESC
) TO 'error_report.csv' WITH (HEADER, DELIMITER ',');
```

### Weekly Summary

```sql
-- Export weekly metrics
COPY (
    SELECT 
        DATE_TRUNC('week', timestamp) as week_start,
        COUNT(*) as total_logs,
        COUNT(*) FILTER (WHERE level = 'error') as errors,
        COUNT(*) FILTER (WHERE level = 'warn') as warnings,
        COUNT(DISTINCT json_extract_string(message, '$.service')) as services,
        ROUND(100.0 * COUNT(*) FILTER (WHERE level = 'error') / COUNT(*), 2) as error_rate
    FROM logs
    WHERE timestamp >= CURRENT_DATE - INTERVAL '90 days'
    GROUP BY week_start
    ORDER BY week_start DESC
) TO 'weekly_summary.csv' WITH (HEADER, DELIMITER ',');
```

---

## Troubleshooting

### Common Errors

#### "No files found"

```
Error: No files found that match the pattern
```

**Solution:** Check S3 credentials and bucket name
```sql
-- Verify S3 settings
SELECT current_setting('s3_access_key_id');
SELECT current_setting('s3_region');

-- Test with simple query
SELECT COUNT(*) FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*');
```

#### "Hive partition mismatch"

```
Error: Hive partition column not found
```

**Solution:** Ensure `hive_partitioning=true` in query
```sql
-- Correct
SELECT * FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*', hive_partitioning=true);
```

#### "Could not connect to S3"

**MinIO:** Check endpoint and SSL settings
```sql
SET s3_endpoint='localhost:9000';
SET s3_use_ssl=false;
```

**AWS S3:** Don't set endpoint for AWS
```sql
-- For AWS S3, only set:
SET s3_region='us-east-1';
SET s3_access_key_id='your-key';
SET s3_secret_access_key='your-secret';
```

### Performance Issues

**Query taking too long?**

1. Check if partition filters are used:
   ```sql
   EXPLAIN SELECT * FROM logs WHERE message LIKE '%error%';
   -- Look for "PARQUET_SCAN" and check files scanned
   ```

2. Add date filter:
   ```sql
   -- Much faster
   SELECT * FROM logs 
   WHERE date = '2024-01-15' 
     AND message LIKE '%error%';
   ```

3. Use LIMIT for testing:
   ```sql
   SELECT * FROM logs LIMIT 100;
   ```

### Checking Data

```sql
-- Verify logs are ingested
SELECT 
    MIN(date) as earliest_date,
    MAX(date) as latest_date,
    COUNT(*) as total_logs,
    COUNT(DISTINCT level) as distinct_levels
FROM logs;

-- Check partition distribution
SELECT 
    date,
    level,
    COUNT(*) as log_count
FROM logs
GROUP BY date, level
ORDER BY date DESC, level;
```

---

## Quick Reference

### Essential Queries

```sql
-- Recent errors
SELECT * FROM logs 
WHERE level = 'error' AND date >= CURRENT_DATE - 7
ORDER BY timestamp DESC LIMIT 20;

-- Error rate today
SELECT 
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE level = 'error') as errors,
    ROUND(100.0 * COUNT(*) FILTER (WHERE level = 'error') / COUNT(*), 2) as error_rate
FROM logs WHERE date = CURRENT_DATE;

-- Most common errors
SELECT message, COUNT(*) as count
FROM logs 
WHERE level = 'error' AND date = CURRENT_DATE
GROUP BY message ORDER BY count DESC LIMIT 10;

-- Search logs
SELECT timestamp, level, message 
FROM logs 
WHERE date = CURRENT_DATE AND message LIKE '%search_term%'
ORDER BY timestamp DESC;
```

### File Organization

```
s3://blobsearch/logs/
├── date=2024-01-15/
│   ├── level=error/
│   │   └── logs_2024-01-15_10_1705318800_batch0000.parquet
│   ├── level=info/
│   │   └── logs_2024-01-15_10_1705318800_batch0001.parquet
│   └── level=warn/
│       └── logs_2024-01-15_10_1705318800_batch0002.parquet
└── date=2024-01-16/
    └── level=error/
        └── logs_2024-01-16_11_1705405200_batch0000.parquet
```

### DuckDB JSON Functions

```sql
-- Extract string
json_extract_string(message, '$.field')

-- Extract number
CAST(json_extract_string(message, '$.count') AS INTEGER)

-- Extract nested
json_extract_string(message, '$.metadata.user.id')

-- Check if field exists
WHERE message LIKE '%"field_name"%'
```

---

## Additional Resources

- **README.md** - Setup and configuration
- **harness/README.md** - Local development environment
- **DuckDB JSON Functions** - https://duckdb.org/docs/extensions/json
- **Parquet Format** - https://parquet.apache.org/