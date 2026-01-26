# Build the ingestor binary
BINARY_NAME=ingestor
BUILD_DIR=bin
GO_FLAGS=-ldflags="-s -w"
CGO_ENABLED=0

.PHONY: all build clean test help

all: build

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/ingestor

build-linux-amd64:
	@echo "Building $(BINARY_NAME) for linux/amd64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/ingestor

build-linux-arm64:
	@echo "Building $(BINARY_NAME) for linux/arm64..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GO_FLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/ingestor

build-all: build-linux-amd64 build-linux-arm64

test:
	go test -v ./...

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)

help:
	@echo "Available targets:"
	@echo "  make build              - Build binary for current platform"
	@echo "  make build-linux-amd64  - Build for Linux amd64"
	@echo "  make build-linux-arm64  - Build for Linux arm64"
	@echo "  make build-all          - Build all platforms"
	@echo "  make test               - Run tests"
	@echo "  make clean              - Remove build artifacts"
