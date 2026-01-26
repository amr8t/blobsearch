// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/parquet-go/parquet-go"
)

var (
	bucket        = flag.String("bucket", "", "S3 bucket name or local directory")
	prefix        = flag.String("prefix", "logs", "S3 prefix for log files")
	batchSize     = flag.Int("batch-size", 10000, "Number of log entries per parquet file")
	compression   = flag.String("compression", "snappy", "Compression algorithm (snappy, gzip, none)")
	localFile     = flag.Bool("local", false, "Write to local files instead of S3")
	logTimestamps = flag.Bool("with-timestamps", false, "Parse and include timestamps from logs")
	endpoint      = flag.String("endpoint", "", "Custom S3 endpoint (for MinIO/local S3)")
	accessKey     = flag.String("access-key", "", "AWS access key (for custom endpoint)")
	secretKey     = flag.String("secret-key", "", "AWS secret key (for custom endpoint)")
	region        = flag.String("region", "us-east-1", "AWS region")
	httpMode      = flag.Bool("http", false, "Run as HTTP server")
	httpPort      = flag.String("port", "8080", "HTTP server port")
	deduplicate   = flag.Bool("deduplicate", false, "Enable deduplication (keeps only unique logs)")
	dedupWindow   = flag.Int("dedup-window", 100000, "Number of recent hashes to keep for deduplication")
)

// LogEntry represents a log entry that will be written to Parquet
type LogEntry struct {
	Timestamp   time.Time `parquet:"timestamp"`
	Message     string    `parquet:"message"`
	Level       string    `parquet:"level"`
	LineNumber  int64     `parquet:"line_number"`
	ContentHash string    `parquet:"content_hash"`
}

// BatchInfo tracks information about the current batch
type BatchInfo struct {
	Entries     []LogEntry
	StartTime   time.Time
	EndTime     time.Time
	LineNumber  int64
	BatchNumber int
}

// PartitionTracker manages partition information for efficient querying
type PartitionTracker struct {
	mu           sync.RWMutex
	partitionMap map[string]int
}

// GetPartitionKey returns the partition key for a log entry
func GetPartitionKey(entry LogEntry) string {
	dateStr := entry.Timestamp.Format("2006-01-02")
	level := entry.Level
	var parts []string
	if dateStr != "" {
		parts = append(parts, fmt.Sprintf("date=%s", dateStr))
	}
	if level != "" && level != "unknown" {
		parts = append(parts, fmt.Sprintf("level=%s", level))
	}
	if len(parts) > 0 {
		return strings.Join(parts, "/")
	}
	return ""
}

// NewPartitionTracker creates a new partition tracker
func NewPartitionTracker() *PartitionTracker {
	return &PartitionTracker{
		partitionMap: make(map[string]int),
	}
}

// UpdatePartition tracks partition usage for a log entry
func (pt *PartitionTracker) UpdatePartition(entry LogEntry) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	partitionKey := GetPartitionKey(entry)
	if partitionKey != "" {
		pt.partitionMap[partitionKey] = 1
	}
}

// GetPartitionCount returns the number of unique partitions
func (pt *PartitionTracker) GetPartitionCount() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.partitionMap)
}

// DedupCache manages a sliding window of content hashes for deduplication
type DedupCache struct {
	mu      sync.RWMutex
	hashes  map[string]bool
	order   []string
	maxSize int
}

func NewDedupCache(maxSize int) *DedupCache {
	return &DedupCache{
		hashes:  make(map[string]bool),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

func (dc *DedupCache) Contains(hash string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.hashes[hash]
}

func (dc *DedupCache) Add(hash string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// If already exists, don't add again
	if dc.hashes[hash] {
		return
	}

	// Add to cache
	dc.hashes[hash] = true
	dc.order = append(dc.order, hash)

	// If cache is full, remove oldest entry
	if len(dc.order) > dc.maxSize {
		oldest := dc.order[0]
		delete(dc.hashes, oldest)
		dc.order = dc.order[1:]
	}
}

func (dc *DedupCache) Size() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return len(dc.hashes)
}

// LogIngestor handles log ingestion with buffering
type LogIngestor struct {
	partitionTracker *PartitionTracker
	s3Client         *s3.Client
	batch            *BatchInfo
	batchNumber      int
	lineCount        int64
	dedupCache       *DedupCache
	duplicateCount   int64
	mu               sync.Mutex
}

func NewLogIngestor(s3Client *s3.Client) *LogIngestor {
	var dedupCache *DedupCache
	if *deduplicate {
		dedupCache = NewDedupCache(*dedupWindow)
		log.Printf("Deduplication enabled (window size: %d)", *dedupWindow)
	}

	return &LogIngestor{
		partitionTracker: NewPartitionTracker(),
		s3Client:         s3Client,
		batch: &BatchInfo{
			Entries:     make([]LogEntry, 0, *batchSize),
			StartTime:   time.Now(),
			EndTime:     time.Now(),
			BatchNumber: 0,
		},
		batchNumber:    0,
		lineCount:      0,
		dedupCache:     dedupCache,
		duplicateCount: 0,
	}
}

func (li *LogIngestor) computeContentHash(message string, timestamp time.Time) string {
	h := sha256.New()
	h.Write([]byte(message))
	h.Write([]byte(timestamp.Format(time.RFC3339Nano)))
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func (li *LogIngestor) ProcessLine(line string) error {
	li.mu.Lock()
	defer li.mu.Unlock()

	li.lineCount++

	// Parse timestamp if enabled
	var timestamp time.Time
	if *logTimestamps {
		timestamp = parseTimestamp(line)
	} else {
		timestamp = time.Now()
	}

	// Compute content hash for deduplication
	contentHash := li.computeContentHash(line, timestamp)

	// Check for duplicates if deduplication is enabled
	if *deduplicate && li.dedupCache != nil {
		if li.dedupCache.Contains(contentHash) {
			li.duplicateCount++
			return nil // Skip duplicate
		}
		li.dedupCache.Add(contentHash)
	}

	// Extract log level from the message
	level := extractLevel(line)

	// Create log entry
	entry := LogEntry{
		Timestamp:   timestamp,
		Message:     line,
		Level:       level,
		LineNumber:  li.lineCount,
		ContentHash: contentHash,
	}

	// Track partition for this entry
	li.partitionTracker.UpdatePartition(entry)

	// Update batch time range
	if timestamp.Before(li.batch.StartTime) {
		li.batch.StartTime = timestamp
	}
	if timestamp.After(li.batch.EndTime) {
		li.batch.EndTime = timestamp
	}

	li.batch.Entries = append(li.batch.Entries, entry)

	// Flush batch if full
	if len(li.batch.Entries) >= *batchSize {
		if err := li.flushBatch(); err != nil {
			return fmt.Errorf("error flushing batch: %w", err)
		}
	}

	return nil
}

func (li *LogIngestor) flushBatch() error {
	if len(li.batch.Entries) == 0 {
		return nil
	}

	if err := flushBatch(li.batch, li.s3Client); err != nil {
		return err
	}

	li.batchNumber++
	li.batch = &BatchInfo{
		Entries:     make([]LogEntry, 0, *batchSize),
		StartTime:   time.Now(),
		EndTime:     time.Now(),
		BatchNumber: li.batchNumber,
	}

	return nil
}

func (li *LogIngestor) Flush() error {
	li.mu.Lock()
	defer li.mu.Unlock()
	return li.flushBatch()
}

func (li *LogIngestor) GetStats() (lineCount int64, partitionCount int, duplicateCount int64, uniqueCount int64) {
	li.mu.Lock()
	defer li.mu.Unlock()
	uniqueCount = li.lineCount - li.duplicateCount
	return li.lineCount, li.partitionTracker.GetPartitionCount(), li.duplicateCount, uniqueCount
}

func main() {
	flag.Parse()

	if *bucket == "" {
		fmt.Println("Error: bucket name is required")
		os.Exit(1)
	}

	// Create S3 client
	var s3Client *s3.Client
	if !*localFile {
		var cfg aws.Config
		var err error

		if *endpoint != "" {
			cfg, err = config.LoadDefaultConfig(context.TODO(),
				config.WithRegion(*region),
			)
			if err != nil {
				log.Fatalf("Failed to load AWS config: %v", err)
			}
		} else {
			cfg, err = config.LoadDefaultConfig(context.TODO())
			if err != nil {
				log.Fatalf("Failed to load AWS config: %v", err)
			}
		}

		s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			if *endpoint != "" {
				o.BaseEndpoint = aws.String(*endpoint)
				o.UsePathStyle = true

				if *accessKey != "" && *secretKey != "" {
					o.Credentials = aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
						return aws.Credentials{
							AccessKeyID:     *accessKey,
							SecretAccessKey: *secretKey,
						}, nil
					})
				}
			}
		})
	}

	// Create output directory if local
	if *localFile {
		if err := os.MkdirAll(*bucket, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	if *httpMode {
		runHTTPServer(s3Client)
	} else {
		runStdinMode(s3Client)
	}
}

func runHTTPServer(s3Client *s3.Client) {
	ingestor := NewLogIngestor(s3Client)

	// Start GELF TCP server in a goroutine (more reliable than UDP)
	go func() {
		if err := StartGELFTCPServer(":12201", ingestor); err != nil {
			log.Fatalf("Failed to start GELF TCP server: %v", err)
		}
	}()

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// Process each line
		scanner := bufio.NewScanner(bytes.NewReader(body))
		linesProcessed := 0
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			if err := ingestor.ProcessLine(line); err != nil {
				log.Printf("Error processing line: %v", err)
				http.Error(w, "Error processing logs", http.StatusInternalServerError)
				return
			}
			linesProcessed++
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Error scanning input: %v", err)
			http.Error(w, "Error scanning input", http.StatusInternalServerError)
			return
		}

		lineCount, partitionCount, duplicateCount, uniqueCount := ingestor.GetStats()
		response := map[string]interface{}{
			"status":          "ok",
			"lines_processed": linesProcessed,
			"total_lines":     lineCount,
			"partitions":      partitionCount,
			"unique_lines":    uniqueCount,
		}
		if *deduplicate {
			response["duplicates_skipped"] = duplicateCount
			response["dedup_cache_size"] = ingestor.dedupCache.Size()
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/flush", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := ingestor.Flush(); err != nil {
			log.Printf("Error flushing: %v", err)
			http.Error(w, "Error flushing", http.StatusInternalServerError)
			return
		}

		lineCount, partitionCount, duplicateCount, uniqueCount := ingestor.GetStats()
		response := map[string]interface{}{
			"status":       "flushed",
			"total_lines":  lineCount,
			"unique_lines": uniqueCount,
			"partitions":   partitionCount,
		}
		if *deduplicate {
			response["duplicates_skipped"] = duplicateCount
			response["dedup_cache_size"] = ingestor.dedupCache.Size()
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		lineCount, partitionCount, duplicateCount, uniqueCount := ingestor.GetStats()
		response := map[string]interface{}{
			"total_lines":  lineCount,
			"unique_lines": uniqueCount,
			"partitions":   partitionCount,
		}
		if *deduplicate {
			response["duplicates_skipped"] = duplicateCount
			response["dedup_cache_size"] = ingestor.dedupCache.Size()
			response["dedup_enabled"] = true
		} else {
			response["dedup_enabled"] = false
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	addr := ":" + *httpPort
	// GELF endpoint for Docker GELF logging driver
	http.HandleFunc("/gelf", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Read and potentially decompress body
		var reader io.Reader = r.Body
		contentEncoding := r.Header.Get("Content-Encoding")

		switch contentEncoding {
		case "gzip":
			gzReader, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Error decompressing gzip", http.StatusBadRequest)
				return
			}
			defer gzReader.Close()
			reader = gzReader
		case "deflate":
			zlibReader, err := zlib.NewReader(r.Body)
			if err != nil {
				http.Error(w, "Error decompressing deflate", http.StatusBadRequest)
				return
			}
			defer zlibReader.Close()
			reader = zlibReader
		}

		body, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, "Error reading body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// GELF can be sent as individual JSON objects or newline-delimited
		scanner := bufio.NewScanner(bytes.NewReader(body))
		linesProcessed := 0

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var gelfMsg GELFMessage
			if err := json.Unmarshal([]byte(line), &gelfMsg); err != nil {
				log.Printf("Error parsing GELF message: %v", err)
				continue
			}

			if err := ingestor.ProcessGELF(gelfMsg); err != nil {
				log.Printf("Error processing GELF: %v", err)
				continue
			}
			linesProcessed++
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Error scanning GELF input: %v", err)
			http.Error(w, "Error scanning input", http.StatusInternalServerError)
			return
		}

		lineCount, partitionCount, duplicateCount, uniqueCount := ingestor.GetStats()
		response := map[string]interface{}{
			"status":          "ok",
			"lines_processed": linesProcessed,
			"total_lines":     lineCount,
			"partitions":      partitionCount,
			"unique_lines":    uniqueCount,
		}
		if *deduplicate {
			response["duplicates_skipped"] = duplicateCount
			response["dedup_cache_size"] = ingestor.dedupCache.Size()
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	})

	log.Printf("Starting HTTP ingestor on %s", addr)
	log.Printf("GELF TCP server on :12201")
	log.Printf("POST logs to http://localhost%s/ingest", addr)
	log.Printf("POST GELF logs to http://localhost%s/gelf", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runStdinMode(s3Client *s3.Client) {
	partitionTracker := NewPartitionTracker()

	// Initialize batch
	batchNumber := 0
	batch := &BatchInfo{
		Entries:     make([]LogEntry, 0, *batchSize),
		StartTime:   time.Now(),
		EndTime:     time.Now(),
		BatchNumber: batchNumber,
	}

	// Read from stdin
	scanner := bufio.NewScanner(os.Stdin)
	lineCount := int64(0)

	fmt.Println("Starting log ingestion...")
	fmt.Println("Reading from stdin, press Ctrl+D to finish...")

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Parse timestamp if enabled
		var timestamp time.Time
		if *logTimestamps {
			timestamp = parseTimestamp(line)
		} else {
			timestamp = time.Now()
		}

		// Compute content hash
		h := sha256.New()
		h.Write([]byte(line))
		h.Write([]byte(timestamp.Format(time.RFC3339Nano)))
		contentHash := fmt.Sprintf("%x", h.Sum(nil))[:16]

		// Extract log level
		level := extractLevel(line)

		// Create log entry
		entry := LogEntry{
			Timestamp:   timestamp,
			Message:     line,
			Level:       level,
			LineNumber:  lineCount,
			ContentHash: contentHash,
		}

		// Track partition for this entry
		partitionTracker.UpdatePartition(entry)

		// Update batch time range
		if timestamp.Before(batch.StartTime) {
			batch.StartTime = timestamp
		}
		if timestamp.After(batch.EndTime) {
			batch.EndTime = timestamp
		}

		batch.Entries = append(batch.Entries, entry)

		// Flush batch if full
		if len(batch.Entries) >= *batchSize {
			if err := flushBatch(batch, s3Client); err != nil {
				log.Printf("Error flushing batch: %v", err)
			}
			batchNumber++
			batch = &BatchInfo{
				Entries:     make([]LogEntry, 0, *batchSize),
				StartTime:   time.Now(),
				EndTime:     time.Now(),
				BatchNumber: batchNumber,
			}
		}

		if lineCount%10000 == 0 {
			fmt.Printf("Processed %d lines...\n", lineCount)
		}
	}

	// Flush remaining entries
	if len(batch.Entries) > 0 {
		if err := flushBatch(batch, s3Client); err != nil {
			log.Printf("Error flushing final batch: %v", err)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Fatalf("Error reading input: %v", err)
	}

	fmt.Printf("\nIngestion complete!\n")
	fmt.Printf("Total lines processed: %d\n", lineCount)
	fmt.Printf("Total partitions created: %d\n", partitionTracker.GetPartitionCount())
}

func flushBatch(batch *BatchInfo, s3Client *s3.Client) error {
	// Group entries by partition key
	partitionGroups := make(map[string][]LogEntry)
	for _, entry := range batch.Entries {
		partitionKey := GetPartitionKey(entry)
		if partitionKey == "" {
			partitionKey = "unpartitioned"
		}
		partitionGroups[partitionKey] = append(partitionGroups[partitionKey], entry)
	}

	// Process each partition group
	partitionCounter := 0
	for partitionKey, entries := range partitionGroups {
		// Generate filename
		baseFileName := generateFileName(batch.StartTime, batch.EndTime, batch.BatchNumber)
		if len(partitionGroups) > 1 {
			baseFileName = fmt.Sprintf("%s_part%d", baseFileName, partitionCounter)
		}
		partitionCounter++

		var fileName string
		if partitionKey != "unpartitioned" {
			fileName = fmt.Sprintf("%s/%s", partitionKey, baseFileName)
		} else {
			fileName = baseFileName
		}

		// Create parquet writer
		var buf bytes.Buffer
		writer := parquet.NewGenericWriter[LogEntry](&buf, getCompression()...)

		// Write entries for this partition
		_, err := writer.Write(entries)
		if err != nil {
			return fmt.Errorf("error writing to parquet: %w", err)
		}

		if err := writer.Close(); err != nil {
			return fmt.Errorf("error closing parquet writer: %w", err)
		}

		data := buf.Bytes()

		// Upload to S3 or write locally
		if *localFile {
			// Write to local file
			localPath := fmt.Sprintf("%s/%s/%s", *bucket, *prefix, fileName)
			dir := localPath[:strings.LastIndex(localPath, "/")]
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("error creating directory: %w", err)
			}
			if err := os.WriteFile(localPath, data, 0644); err != nil {
				return fmt.Errorf("error writing local file: %w", err)
			}
			log.Printf("Wrote %d entries to %s (%d bytes)\n", len(entries), localPath, len(data))
		} else {
			// Upload to S3
			key := fmt.Sprintf("%s/%s", *prefix, fileName)
			_, err := s3Client.PutObject(context.TODO(), &s3.PutObjectInput{
				Bucket: aws.String(*bucket),
				Key:    aws.String(key),
				Body:   bytes.NewReader(data),
			})
			if err != nil {
				return fmt.Errorf("error uploading to S3: %w", err)
			}
			log.Printf("Uploaded %d entries to s3://%s/%s (%d bytes)\n", len(entries), *bucket, key, len(data))
		}
	}

	return nil
}

func extractLevel(message string) string {
	messageLower := strings.ToLower(message)

	// Check for JSON structured logs
	if strings.HasPrefix(message, "{") && strings.Contains(message, `"level"`) {
		levelPattern := regexp.MustCompile(`"level"\s*:\s*"([^"]+)"`)
		matches := levelPattern.FindStringSubmatch(message)
		if len(matches) > 1 {
			return strings.ToLower(matches[1])
		}
	}

	// Check for common log level patterns in unstructured logs
	if strings.Contains(messageLower, "error") || strings.Contains(messageLower, "[error]") {
		return "error"
	}
	if strings.Contains(messageLower, "warn") || strings.Contains(messageLower, "[warn]") {
		return "warn"
	}
	if strings.Contains(messageLower, "notice") || strings.Contains(messageLower, "info") || strings.Contains(messageLower, "[info]") {
		return "info"
	}
	if strings.Contains(messageLower, "debug") || strings.Contains(messageLower, "[debug]") {
		return "debug"
	}
	return "unknown"
}

func generateFileName(start, end time.Time, batchNum int) string {
	dateStr := start.Format("2006-01-02")
	hour := start.Format("15")
	startSec := start.Unix()
	return fmt.Sprintf("logs_%s_%s_%d_batch%04d.parquet", dateStr, hour, startSec, batchNum)
}

func getCompression() []parquet.WriterOption {
	switch strings.ToLower(*compression) {
	case "snappy":
		return []parquet.WriterOption{parquet.Compression(&parquet.Snappy)}
	case "gzip":
		return []parquet.WriterOption{parquet.Compression(&parquet.Gzip)}
	case "none":
		return nil
	default:
		return []parquet.WriterOption{parquet.Compression(&parquet.Snappy)}
	}
}

func parseTimestamp(logLine string) time.Time {
	// Try JSON timestamp extraction first (Web App logs)
	// Format: {"timestamp":"2024-06-03T13:02:04Z",...}
	if strings.HasPrefix(logLine, "{") && strings.Contains(logLine, `"timestamp"`) {
		timestampPattern := regexp.MustCompile(`"timestamp":"([^"]+)"`)
		matches := timestampPattern.FindStringSubmatch(logLine)
		if len(matches) > 1 {
			timestampStr := matches[1]
			// Try RFC3339 formats
			formats := []string{time.RFC3339, time.RFC3339Nano}
			for _, format := range formats {
				if t, err := time.Parse(format, timestampStr); err == nil {
					if t.Year() > 2000 && t.Year() < 2100 {
						return t
					}
				}
			}
		}
	}

	// Extract timestamp from Apache log format: [Day Mon DD HH:MM:SS YYYY]
	if strings.Contains(logLine, "[") && strings.Contains(logLine, "]") {
		start := strings.Index(logLine, "[")
		end := strings.Index(logLine, "]")
		if end > start {
			timestampStr := logLine[start+1 : end]

			// Apache log format: Mon Jan 02 15:04:05 2006
			format := "Mon Jan 02 15:04:05 2006"
			if t, err := time.Parse(format, timestampStr); err == nil {
				if t.Year() > 2000 && t.Year() < 2100 {
					return t
				}
			}
		}
	}

	// Fallback: try other common formats at start of line
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"02/Jan/2006:15:04:05 -0700",
	}

	for _, format := range formats {
		if len(logLine) >= len(format) {
			potential := logLine[:len(format)]
			if t, err := time.Parse(format, potential); err == nil {
				if t.Year() > 2000 && t.Year() < 2100 {
					return t
				}
			}
		}
	}

	// Last resort: use current time
	return time.Now()
}
