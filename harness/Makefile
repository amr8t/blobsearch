# BlobSearch Makefile

.PHONY: help build test clean docker-up docker-down docker-logs \
	generate-logs load-logs flush query stats stream-start stream-stop stream-logs

.DEFAULT_GOAL := help

# Colors
BLUE := \033[0;34m
GREEN := \033[0;32m
YELLOW := \033[0;33m
NC := \033[0m

help: ## Show this help message
	@printf "$(BLUE)BlobSearch - Log Analytics System$(NC)\n\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-18s$(NC) %s\n", $$1, $$2}'

## Core Commands

build: ## Build ingestor binary
	@printf "$(BLUE)Building ingestor...$(NC)\n"
	cd .. && go build -o harness/bin/ingestor ./cmd/ingestor
	@printf "$(GREEN)Built: bin/ingestor$(NC)\n"

test: ## Run tests
	@printf "$(BLUE)Running tests...$(NC)\n"
	cd .. && go test -v ./...

clean: ## Clean build artifacts and data
	@printf "$(BLUE)Cleaning up...$(NC)\n"
	rm -rf bin/ ../generated-*.log
	@cd ../examples/docker && docker-compose down 2>/dev/null || true
	docker-compose down -v
	@printf "$(GREEN)Cleaned!$(NC)\n"

## Docker Services

docker-up: ## Start BlobSearch services (MinIO + Ingestor)
	@printf "$(BLUE)Starting BlobSearch...$(NC)\n"
	docker-compose up -d
	@printf "$(GREEN)Services started!$(NC)\n"
	@printf "$(YELLOW)MinIO:    http://localhost:9001 (minioadmin/minioadmin)$(NC)\n"
	@printf "$(YELLOW)Ingestor: http://localhost:8080$(NC)\n"

docker-down: ## Stop all services
	@printf "$(BLUE)Stopping services...$(NC)\n"
	docker-compose down

docker-logs: ## Show service logs
	docker-compose logs -f

## Log Generation & Loading

generate-logs: ## Generate sample JSON logs (count=1000 days=7)
	@if [ ! -f generator/main.go ]; then \
		printf "$(YELLOW)Error: generator not found at generator/$(NC)\n"; \
		exit 1; \
	fi
	@printf "$(BLUE)Generating JSON logs...$(NC)\n"
	@cd generator && go run main.go \
		-count $(or $(count),1000) \
		-days $(or $(days),7) \
		-output ../../generated-logs.json
	@printf "$(GREEN)Generated: ../generated-logs.json$(NC)\n"

load-logs: ## Load logs from file (file=path/to/logs.log)
	@if [ -z "$(file)" ]; then \
		printf "$(YELLOW)Usage: make load-logs file=path/to/logs.log$(NC)\n"; \
		exit 1; \
	fi
	@printf "$(BLUE)Loading $(file)...$(NC)\n"
	@cat $(file) | curl -X POST --data-binary @- http://localhost:8080/ingest
	@printf "\n$(GREEN)Loaded!$(NC)\n"

flush: ## Flush ingestor buffer to storage
	@printf "$(BLUE)Flushing...$(NC)\n"
	@curl -s -X POST http://localhost:8080/flush && printf "\n$(GREEN)Flushed!$(NC)\n" || printf "\n$(YELLOW)Nothing to flush or already flushed$(NC)\n"

stats: ## Show ingestor statistics
	@curl -s http://localhost:8080/stats | python3 -m json.tool 2>/dev/null || curl -s http://localhost:8080/stats

## Query Commands

query: ## Start DuckDB with BlobSearch connection
	@printf "$(BLUE)Starting DuckDB...$(NC)\n"
	@printf "$(YELLOW)Quick setup:$(NC)\n"
	@printf "  INSTALL httpfs; LOAD httpfs;\n"
	@printf "  SET s3_endpoint='localhost:9000';\n"
	@printf "  SET s3_access_key_id='blobsearch'; SET s3_secret_access_key='blobsearch123';\n"
	@printf "  SET s3_use_ssl=false; SET s3_url_style='path';\n"
	@printf "  CREATE VIEW logs AS SELECT * FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*', hive_partitioning=true);\n\n"
	@duckdb

query-stats: ## Quick query: show log statistics
	@printf "$(BLUE)Querying statistics...$(NC)\n"
	@curl -s -X POST http://localhost:8080/flush >/dev/null 2>&1 || true
	@duckdb -c "\
		INSTALL httpfs; LOAD httpfs; \
		SET s3_endpoint='localhost:9000'; \
		SET s3_access_key_id='blobsearch'; SET s3_secret_access_key='blobsearch123'; \
		SET s3_use_ssl=false; SET s3_url_style='path'; \
		CREATE VIEW logs AS SELECT * FROM read_parquet('s3://blobsearch/logs/date=*/level=*/*', hive_partitioning=true); \
		SELECT COUNT(*) as total_logs, \
			COUNT(DISTINCT level) as unique_levels, \
			COUNT(DISTINCT date) as unique_dates \
		FROM logs;" 2>/dev/null || printf "$(YELLOW)No logs yet. Try: make generate-logs load-logs$(NC)\n"

## Stream Logs (Docker Container)

stream-start: ## Start log generator container (streams to ingestor)
	@printf "$(BLUE)Starting log generator...$(NC)\n"
	docker-compose --profile generator up -d log-generator
	@printf "$(GREEN)Generator started! Streaming logs to ingestor$(NC)\n"
	@printf "$(YELLOW)View logs: make stream-logs$(NC)\n"
	@printf "$(YELLOW)Stop: make stream-stop$(NC)\n"

stream-stop: ## Stop log generator container
	@printf "$(BLUE)Stopping log generator...$(NC)\n"
	docker-compose --profile generator stop log-generator
	docker-compose --profile generator rm -f log-generator
	@printf "$(GREEN)Generator stopped$(NC)\n"

stream-logs: ## Show log generator output
	docker-compose --profile generator logs -f log-generator

stream-manual: ## Stream logs manually (delay=1s batch=10)
	@printf "$(BLUE)Streaming logs to ingestor...$(NC)\n"
	@cd generator && go run main.go -stream \
		-delay $(or $(delay),1s) \
		-endpoint http://localhost:8080/ingest \
		-batch $(or $(batch),10)
