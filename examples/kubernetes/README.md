# Kubernetes Example

**ULTRA SIMPLE** - DaemonSet pattern. One ingestor per node.

## Quick Start with k3s

### 1. Install k3s

```bash
curl -sfL https://get.k3s.io | sh -
```

This installs k3s and kubectl automatically.

### 2. Deploy BlobSearch

```bash
kubectl apply -f blobsearch.yaml
```

This deploys:
- MinIO (local S3)
- BlobSearch ingestor (DaemonSet)
- Example app
- Services

### 3. Create Bucket

```bash
kubectl exec minio -- mc alias set myminio http://localhost:9000 minioadmin minioadmin
kubectl exec minio -- mc mb myminio/logs --ignore-existing
```

### 4. Test Ingestion

```bash
# Send logs via NodePort
echo '{"level":"info","message":"Test from k8s"}' | \
  curl -X POST --data-binary @- http://localhost:30080/ingest

# Check stats
curl http://localhost:30080/stats

# Flush to S3
curl -X POST http://localhost:30080/flush

# Verify in MinIO
kubectl exec minio -- mc ls -r myminio/logs/
```

### 5. Verify Everything

```bash
# Check pods
kubectl get pods

# Check services
kubectl get svc

# Check DaemonSet
kubectl get daemonset
```

## Configuration

### Auto-Flush (Default: Enabled)

Logs are automatically flushed to S3 every 90 seconds by default. The DaemonSet includes:

```yaml
- name: AUTO_FLUSH
  value: "true"
- name: AUTO_FLUSH_INTERVAL
  value: "90"  # seconds
```

To customize, edit `blobsearch.yaml` and change the interval:

```yaml
- name: AUTO_FLUSH_INTERVAL
  value: "30"  # Flush every 30 seconds for near real-time
```

Or disable auto-flush for manual control:

```yaml
- name: AUTO_FLUSH
  value: "false"
```

Then manually flush via the API:
```bash
curl -X POST http://localhost:30080/flush
```

See [Auto-Flush Guide](../AUTO_FLUSH_GUIDE.md) for detailed configuration options.

## Using AWS S3 (Production)

Edit `blobsearch.yaml`:

1. Remove MinIO pod and service
2. Update secret with your AWS credentials:
```yaml
stringData:
  access-key: "your-aws-key"
  secret-key: "your-aws-secret"
```

3. Update ingestor endpoint:
```yaml
- name: ENDPOINT
  value: "https://s3.amazonaws.com"
```

4. Deploy:
```bash
kubectl apply -f blobsearch.yaml
```

## Access Patterns

### NodePort (Simple - What we use)
Ingestor is exposed on port 30080 (HTTP) and 30201 (GELF).

```bash
# From anywhere
curl http://NODE_IP:30080/ingest
```

### Service DNS (Pod-to-Pod)
Other pods can use the service name:

```bash
http://blobsearch-ingestor.default.svc.cluster.local:8080
tcp://blobsearch-ingestor.default.svc.cluster.local:12201
```

### Port Forward (Testing)
```bash
kubectl port-forward daemonset/blobsearch-ingestor 8080:8080
curl http://localhost:8080/stats
```

## GELF Logging from Pods

To send logs from your apps, you need to configure containerd or docker runtime with GELF driver.

**Simple approach:** Use HTTP POST instead:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: myapp
spec:
  containers:
  - name: app
    image: myapp:latest
  - name: log-forwarder
    image: busybox
    command: ["sh", "-c"]
    args:
    - |
      while true; do
        kubectl logs -f myapp app | \
        while read line; do
          echo "$line" | curl -X POST --data-binary @- \
            http://blobsearch-ingestor:8080/ingest
        done
        sleep 1
      done
```

**Better approach:** Use a proper log shipper like Fluent Bit as a DaemonSet that reads container logs and forwards to BlobSearch.

## Query Logs

Same as always:

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

For AWS S3, adjust the settings accordingly.

## Cleanup

```bash
kubectl delete -f blobsearch.yaml
```

## Uninstall k3s

```bash
/usr/local/bin/k3s-uninstall.sh
```

## Multi-Node Cluster

The DaemonSet automatically runs one ingestor per node. Each node's pods can send to `localhost:30201` for GELF logging.

## That's It

- k3s installs in seconds
- DaemonSet = one ingestor per node  
- NodePort = easy access from anywhere
- Auto-flush every 90 seconds (configurable)
- Logs go to S3
- Query with DuckDB

**Key Features:**
- ✓ Auto-flush enabled by default
- ✓ One ingestor per node (DaemonSet)
- ✓ Real S3 or MinIO support
- ✓ Pre-built images from GitHub