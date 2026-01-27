# Standalone Docker Example

**VERY SIMPLE** - Uses pre-built image from GitHub. No building required.

## Quick Start

### 1. Start Infrastructure

```bash
docker-compose up -d
```

This starts:
- MinIO (local S3)
- BlobSearch ingestor

### 2. Create Bucket (First Time Only)

```bash
docker run --rm --network standalone-docker_default \
  --entrypoint /bin/sh minio/mc -c "
    mc alias set myminio http://minio:9000 minioadmin minioadmin && \
    mc mb myminio/logs --ignore-existing
  "
```

### 3. Start Your App with Logging

Get the ingestor IP:

```bash
INGESTOR_IP=$(docker inspect standalone-docker-ingestor-1 --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')
```

Run your app:

```bash
docker run -d \
  --name myapp \
  --network standalone-docker_default \
  --log-driver=gelf \
  --log-opt gelf-address=tcp://${INGESTOR_IP}:12201 \
  --log-opt tag=myapp \
  busybox sh -c "while true; do echo 'Hello from my app'; sleep 2; done"
```

### 4. Verify Logs

```bash
# Check stats
curl http://localhost:8080/stats

# Flush to S3
curl -X POST http://localhost:8080/flush

# View in MinIO console
open http://localhost:9001
# Login: minioadmin / minioadmin
```

## Using AWS S3 Instead of MinIO

Edit `docker-compose.yml`:

```yaml
ingestor:
  environment:
    ENDPOINT: https://s3.amazonaws.com
    ACCESS_KEY: your-aws-key
    SECRET_KEY: your-aws-secret
    BUCKET: your-bucket
    REGION: us-east-1
```

Remove the `minio` service, then:

```bash
docker-compose up -d
```

## Your Own Application

```bash
# Get ingestor IP
INGESTOR_IP=$(docker inspect standalone-docker-ingestor-1 --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}')

# Run your app with GELF logging
docker run -d \
  --name your-app \
  --network standalone-docker_default \
  --log-driver=gelf \
  --log-opt gelf-address=tcp://${INGESTOR_IP}:12201 \
  --log-opt tag=your-app \
  your-image:latest
```

## Alternative: Use Published Port

If networking is simpler with host ports:

```bash
docker run -d \
  --name myapp \
  --log-driver=gelf \
  --log-opt gelf-address=tcp://172.17.0.1:12201 \
  --log-opt tag=myapp \
  busybox sh -c "while true; do echo 'test'; sleep 2; done"
```

`172.17.0.1` is the Docker bridge gateway (host from container perspective).

## Query Logs

```bash
duckdb -c "
  INSTALL httpfs; LOAD httpfs;
  SET s3_endpoint='localhost:9000';
  SET s3_use_ssl=false;
  SET s3_access_key_id='minioadmin';
  SET s3_secret_access_key='minioadmin';
  SELECT * FROM read_parquet('s3://logs/logs/*/*', hive_partitioning=true) LIMIT 10;
"
```

## Cleanup

```bash
docker stop myapp && docker rm myapp
docker-compose down
```

## That's It

Simple standalone setup. Apps use Docker's native GELF logging driver to send logs to BlobSearch.