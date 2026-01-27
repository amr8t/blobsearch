// SPDX-License-Identifier: AGPL-3.0-only

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

var (
	count     = flag.Int("count", 1000, "Number of log entries to generate (ignored in stream mode)")
	output    = flag.String("output", "", "Output file path (writes to stdout if not specified)")
	stream    = flag.Bool("stream", false, "Stream mode: continuously generate logs (Ctrl+C to stop)")
	delay     = flag.Duration("delay", 1*time.Second, "Delay between logs in stream mode (e.g., 100ms, 1s, 2s)")
	startDate = flag.String("start-date", "", "Start date for log timestamps (format: 2006-01-02, default: today)")
	days      = flag.Int("days", 1, "Number of days to span logs across")
	endpoint  = flag.String("endpoint", "", "HTTP endpoint to POST logs to (e.g., http://localhost:8080/ingest)")
	batch     = flag.Int("batch", 1, "Number of logs to batch together before sending (only with -endpoint)")
)

func usage() {
	fmt.Fprintf(os.Stderr, "BlobSearch Log Generator\n\n")
	fmt.Fprintf(os.Stderr, "Generate structured JSON logs for testing BlobSearch ingestion.\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  %s [options]\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Options:\n")
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nExamples:\n")
	fmt.Fprintf(os.Stderr, "  # Generate 1000 JSON logs to stdout\n")
	fmt.Fprintf(os.Stderr, "  %s -count 1000\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # Generate 5000 logs to file\n")
	fmt.Fprintf(os.Stderr, "  %s -count 5000 -output logs.json\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # Generate logs spanning 7 days starting from 2024-01-01\n")
	fmt.Fprintf(os.Stderr, "  %s -count 10000 -start-date 2024-01-01 -days 7\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # Generate logs for the past 30 days\n")
	fmt.Fprintf(os.Stderr, "  %s -count 50000 -days 30 -output logs.json\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # Stream logs continuously with 500ms delay\n")
	fmt.Fprintf(os.Stderr, "  %s -stream -delay 500ms\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # Pipe directly to ingestor\n")
	fmt.Fprintf(os.Stderr, "  %s -count 10000 | curl -X POST --data-binary @- http://localhost:8080/ingest\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # Stream logs directly to HTTP endpoint\n")
	fmt.Fprintf(os.Stderr, "  %s -stream -delay 500ms -endpoint http://localhost:8080/ingest\n\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  # POST logs in batches\n")
	fmt.Fprintf(os.Stderr, "  %s -count 10000 -endpoint http://localhost:8080/ingest -batch 100\n\n", os.Args[0])
}

func main() {
	flag.Usage = usage
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	// Parse date range
	var startTime time.Time
	var err error
	if *startDate != "" {
		startTime, err = time.Parse("2006-01-02", *startDate)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing start-date: %v\n", err)
			fmt.Fprintf(os.Stderr, "Use format: 2006-01-02 (e.g., 2024-01-15)\n")
			os.Exit(1)
		}
	} else {
		// Default to today minus days (so we end at today)
		startTime = time.Now().AddDate(0, 0, -(*days - 1))
	}
	endTime := startTime.AddDate(0, 0, *days)

	// Determine output destination
	var writer = os.Stdout
	if *output != "" {
		f, err := os.Create(*output)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		writer = f
	}

	generator := &LogGenerator{startTime: startTime, endTime: endTime}

	if !*stream {
		fmt.Fprintf(os.Stderr, "Generating JSON logs from %s to %s (%d days)...\n",
			startTime.Format("2006-01-02"), endTime.Format("2006-01-02"), *days)
	} else {
		fmt.Fprintf(os.Stderr, "Generating JSON logs...\n")
	}

	// HTTP endpoint mode
	if *endpoint != "" {
		if *stream {
			streamToHTTP(generator, *endpoint, *delay, *batch)
		} else {
			batchToHTTP(generator, *endpoint, *count, *batch)
		}
		return
	}

	// File/stdout mode
	if *stream {
		// Stream mode: generate logs continuously
		fmt.Fprintf(os.Stderr, "Stream mode: generating logs every %v (Ctrl+C to stop)\n", *delay)
		generated := 0
		for {
			log := generator.Generate()
			fmt.Fprintln(writer, log)
			generated++

			if generated%100 == 0 {
				fmt.Fprintf(os.Stderr, "Generated %d logs...\n", generated)
			}

			time.Sleep(*delay)
		}
	} else {
		// Fixed count mode
		for i := 0; i < *count; i++ {
			log := generator.Generate()
			fmt.Fprintln(writer, log)

			if (i+1)%1000 == 0 {
				fmt.Fprintf(os.Stderr, "Generated %d/%d logs...\n", i+1, *count)
			}
		}
		fmt.Fprintf(os.Stderr, "Successfully generated %d JSON logs\n", *count)
	}
}

// streamToHTTP continuously generates and POSTs logs to HTTP endpoint
func streamToHTTP(generator *LogGenerator, endpoint string, delay time.Duration, batchSize int) {
	fmt.Fprintf(os.Stderr, "Streaming logs to %s every %v (batch size: %d)\n", endpoint, delay, batchSize)

	client := &http.Client{Timeout: 10 * time.Second}
	generated := 0
	buffer := &bytes.Buffer{}

	for {
		// Generate batch
		for i := 0; i < batchSize; i++ {
			log := generator.Generate()
			buffer.WriteString(log)
			buffer.WriteString("\n")
		}

		// POST to endpoint
		resp, err := client.Post(endpoint, "application/json", buffer)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error posting to %s: %v\n", endpoint, err)
		} else {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			generated += batchSize

			if generated%100 == 0 {
				fmt.Fprintf(os.Stderr, "Posted %d logs to %s\n", generated, endpoint)
			}
		}

		buffer.Reset()
		time.Sleep(delay)
	}
}

// batchToHTTP generates fixed count of logs and POSTs in batches
func batchToHTTP(generator *LogGenerator, endpoint string, count, batchSize int) {
	fmt.Fprintf(os.Stderr, "Posting %d logs to %s (batch size: %d)\n", count, endpoint, batchSize)

	client := &http.Client{Timeout: 30 * time.Second}
	buffer := &bytes.Buffer{}
	posted := 0

	for i := 0; i < count; i++ {
		log := generator.Generate()
		buffer.WriteString(log)
		buffer.WriteString("\n")

		// Send batch when full or at end
		if (i+1)%batchSize == 0 || i == count-1 {
			resp, err := client.Post(endpoint, "application/json", buffer)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error posting to %s: %v\n", endpoint, err)
			} else {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				posted += buffer.Len()
			}
			buffer.Reset()

			if (i+1)%1000 == 0 {
				fmt.Fprintf(os.Stderr, "Posted %d/%d logs...\n", i+1, count)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "Successfully posted %d logs to %s\n", count, endpoint)
}

// LogGenerator generates OpenTelemetry-compliant structured JSON logs
type LogGenerator struct {
	startTime time.Time
	endTime   time.Time
}

func (g *LogGenerator) Generate() string {
	var timestamp time.Time
	if !g.startTime.IsZero() {
		// Generate random timestamp within the date range
		timestamp = randomTime(g.startTime, g.endTime)
	} else {
		timestamp = time.Now()
	}

	pattern := webAppPatterns[rand.Intn(len(webAppPatterns))]
	traceID := generateTraceID()
	spanID := generateSpanID()

	// Map level to OpenTelemetry severity
	severityMap := map[string]int{
		"debug": 5,  // DEBUG
		"info":  9,  // INFO
		"warn":  13, // WARN
		"error": 17, // ERROR
	}
	severityNumber := severityMap[pattern.Level]
	if severityNumber == 0 {
		severityNumber = 9 // Default to INFO
	}

	// Build attributes
	attributes := make(map[string]interface{})

	// Add HTTP attributes if applicable
	if rand.Float32() < 0.7 {
		attributes["http.method"] = randomChoice(httpMethods)
		attributes["http.route"] = randomChoice(endpoints)
		attributes["http.status_code"] = statusCodes[rand.Intn(len(statusCodes))]
		attributes["http.request_id"] = generateRequestID()
		attributes["http.user_id"] = fmt.Sprintf("user_%d", rand.Intn(10000))
		attributes["http.duration_ms"] = rand.Intn(5000)
	}

	// Add error attributes
	if pattern.Level == "error" {
		attributes["error.type"] = randomChoice(errorCodes)
		attributes["exception.message"] = randomChoice(errorMessages)
		if rand.Float32() < 0.6 {
			attributes["exception.stacktrace"] = generateStackTrace()
		}
	}

	// Add database attributes
	if rand.Float32() < 0.3 {
		attributes["db.system"] = randomChoice(databases)
		attributes["db.operation"] = randomChoice([]string{"SELECT", "INSERT", "UPDATE", "DELETE"})
	}

	// OpenTelemetry log record structure
	logEntry := map[string]interface{}{
		"timestamp":         timestamp.Format(time.RFC3339Nano),
		"observedTimestamp": timestamp.Format(time.RFC3339Nano),
		"severityNumber":    severityNumber,
		"severityText":      strings.ToUpper(pattern.Level),
		"body":              g.formatMessage(pattern.Template),
		"traceId":           traceID,
		"spanId":            spanID,
		"resource": map[string]interface{}{
			"service.name":           randomChoice(services),
			"service.version":        fmt.Sprintf("1.%d.%d", rand.Intn(10), rand.Intn(20)),
			"deployment.environment": randomChoice([]string{"production", "staging", "development"}),
		},
		"attributes": attributes,
	}

	// Convert to JSON
	jsonBytes, _ := json.Marshal(logEntry)
	return string(jsonBytes)
}

func (g *LogGenerator) formatMessage(template string) string {
	replacements := map[string]string{
		"{user_id}":    fmt.Sprintf("user_%d", rand.Intn(10000)),
		"{endpoint}":   randomChoice(endpoints),
		"{method}":     randomChoice(httpMethods),
		"{status}":     fmt.Sprintf("%d", statusCodes[rand.Intn(len(statusCodes))]),
		"{duration}":   fmt.Sprintf("%d", rand.Intn(5000)),
		"{error}":      randomChoice(errorMessages),
		"{ip}":         generateIP(),
		"{count}":      fmt.Sprintf("%d", rand.Intn(1000)),
		"{threshold}":  fmt.Sprintf("%d", rand.Intn(100)),
		"{database}":   randomChoice(databases),
		"{queue}":      randomChoice(queues),
		"{cache_key}":  fmt.Sprintf("cache:%s:%d", randomChoice(cacheKeys), rand.Intn(10000)),
		"{bytes}":      fmt.Sprintf("%d", rand.Intn(1000000)),
		"{percentage}": fmt.Sprintf("%.2f", rand.Float64()*100),
	}

	result := template
	for k, v := range replacements {
		result = replaceFirst(result, k, v)
	}
	return result
}

// Helper functions

func generateIP() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		rand.Intn(255)+1,
		rand.Intn(256),
		rand.Intn(256),
		rand.Intn(255)+1,
	)
}

func generateRequestID() string {
	return fmt.Sprintf("req_%s", randomString(16))
}

func generateTraceID() string {
	return randomString(32)
}

func generateSpanID() string {
	return randomString(16)
}

func generateStackTrace() string {
	traces := []string{
		"at handleRequest (app.js:145)",
		"at Database.query (db.js:89)",
		"at validateUser (auth.js:234)",
		"at processPayment (payment.js:456)",
		"at sendEmail (email.js:78)",
	}
	numLines := rand.Intn(3) + 2
	result := ""
	for i := 0; i < numLines && i < len(traces); i++ {
		result += traces[i]
		if i < numLines-1 {
			result += " | "
		}
	}
	return result
}

func randomString(length int) string {
	const charset = "abcdef0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

func randomChoice(slice []string) string {
	return slice[rand.Intn(len(slice))]
}

func replaceFirst(s, old, new string) string {
	for i := 0; i < len(s)-len(old)+1; i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

func randomTime(start, end time.Time) time.Time {
	delta := end.Sub(start)
	randomDuration := time.Duration(rand.Int63n(int64(delta)))
	return start.Add(randomDuration)
}

// Log pattern definitions

type LogPattern struct {
	Level    string
	Template string
}

var webAppPatterns = []LogPattern{
	{"info", "Request processed successfully"},
	{"info", "User {user_id} logged in from {ip}"},
	{"info", "API request: {method} {endpoint} completed in {duration}ms"},
	{"info", "Cache hit for key {cache_key}"},
	{"info", "Database query completed in {duration}ms"},
	{"debug", "Processing request for endpoint {endpoint}"},
	{"debug", "Validating request parameters"},
	{"debug", "Cache lookup for key {cache_key}"},
	{"warn", "Slow query detected: {duration}ms for {endpoint}"},
	{"warn", "High memory usage: {percentage}% of limit"},
	{"warn", "Rate limit approaching for user {user_id}"},
	{"warn", "Cache miss rate above threshold: {percentage}%"},
	{"warn", "Queue depth for {queue} is {count} (threshold: {threshold})"},
	{"error", "Database connection failed: {error}"},
	{"error", "Failed to process payment for user {user_id}: {error}"},
	{"error", "Authentication failed for user {user_id} from {ip}"},
	{"error", "Request timeout after {duration}ms for {endpoint}"},
	{"error", "Failed to connect to {database}: connection refused"},
	{"error", "Validation error: {error}"},
	{"error", "Unexpected error in {endpoint}: {error}"},
}

var services = []string{
	"api-gateway", "auth-service", "payment-service", "user-service",
	"notification-service", "order-service", "inventory-service",
}

var endpoints = []string{
	"/api/v1/users", "/api/v1/orders", "/api/v1/products",
	"/api/v1/auth/login", "/api/v1/auth/register", "/api/v1/payments",
	"/api/v1/inventory", "/api/v2/users", "/health", "/metrics",
}

var httpMethods = []string{
	"GET", "POST", "PUT", "DELETE", "PATCH",
}

var statusCodes = []int{
	200, 201, 204, 400, 401, 403, 404, 422, 500, 502, 503,
}

var errorCodes = []string{
	"ERR_DB_CONNECTION", "ERR_TIMEOUT", "ERR_VALIDATION",
	"ERR_AUTH_FAILED", "ERR_NOT_FOUND", "ERR_PAYMENT_FAILED",
	"ERR_RATE_LIMIT", "ERR_INTERNAL",
}

var errorMessages = []string{
	"connection timeout", "validation failed", "user not found",
	"permission denied", "invalid token", "database error",
	"service unavailable", "internal server error",
}

var databases = []string{
	"postgres-primary", "postgres-replica", "redis-cache",
	"mongodb-orders", "elasticsearch",
}

var queues = []string{
	"email-queue", "notification-queue", "analytics-queue",
	"payment-queue", "export-queue",
}

var cacheKeys = []string{
	"user", "session", "product", "inventory", "config",
}
