# Makefile
.PHONY: proto proto-install build build-linux run test coverage vet fmt tidy deps clean clean-all docker docker-build docker-run docker-compose-up docker-compose-down docker-rebuild migrate-up migrate-down help

# Binary name
BINARY_NAME=openmachinecore
OUTPUT_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=$(GOCMD) fmt
GOMOD=$(GOCMD) mod

# Build parameters
MAIN_PATH=cmd/server/main.go
LDFLAGS=-ldflags="-s -w"

# Proto parameters
PROTOC_INCLUDES = -I. \
	-I./third_party/googleapis \
	-I/usr/include

# Default target
all: proto build

# Install protobuf compiler and Go plugins
proto-install:
	@echo "Installing protobuf tools..."
	$(GOCMD) install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	$(GOCMD) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(GOCMD) install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
	@echo "Protobuf tools installed. Make sure protoc is installed:"
	@echo "  - macOS: brew install protobuf"
	@echo "  - Ubuntu: sudo apt-get install protobuf-compiler"

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	protoc $(PROTOC_INCLUDES) \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative \
		api/proto/*.proto
	@echo "Protobuf code generated successfully"

# Clean generated proto files
proto-clean:
	@echo "Cleaning generated protobuf files..."
	rm -f api/proto/*.pb.go
	@echo "Protobuf files cleaned"

# Build the application
build: proto
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Build complete: $(OUTPUT_DIR)/$(BINARY_NAME)"

# Build for Linux (useful for cross-compilation)
build-linux: proto
	@echo "Building $(BINARY_NAME) for Linux..."
	@mkdir -p $(OUTPUT_DIR)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "Linux build complete: $(OUTPUT_DIR)/$(BINARY_NAME)"

# Run the application
run:
	@echo "Running $(BINARY_NAME)..."
	$(GOCMD) run $(MAIN_PATH)

# Run with build
run-build: build
	@echo "Running $(BINARY_NAME)..."
	./$(OUTPUT_DIR)/$(BINARY_NAME)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...
	@echo "Tests complete"

# Run tests with race detection and coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...
	@echo "Tests complete"

# Generate coverage report
coverage: test-coverage
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...
	@echo "Vet complete"

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...
	@echo "Format complete"

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy
	@echo "Tidy complete"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	@echo "Dependencies downloaded"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(OUTPUT_DIR)
	rm -f coverage.txt coverage.html
	@echo "Clean complete"

# Full clean including proto files
clean-all: clean proto-clean
	@echo "Full clean complete"

# Docker build
docker: docker-build

docker-build:
	@echo "Building Docker image..."
	docker build -t $(BINARY_NAME):latest .
	@echo "Docker build complete"

# Docker run
docker-run:
	@echo "Running Docker container..."
	docker run -p 8080:8080 -p 50051:50051 \
		-v $(PWD)/configs:/app/configs \
		$(BINARY_NAME):latest

# Docker compose up
docker-compose-up:
	@echo "Starting Docker Compose..."
	docker-compose up -d
	@echo "Docker Compose started"

# Docker compose down
docker-compose-down:
	@echo "Stopping Docker Compose..."
	docker-compose down
	@echo "Docker Compose stopped"

# Docker full rebuild
docker-rebuild: docker-compose-down
	@echo "Rebuilding Docker images..."
	docker-compose build --no-cache
	docker-compose up -d
	@echo "Docker rebuild complete"

# Database migrations
migrate-up:
	@echo "Running database migrations..."
	psql postgresql://omc:omc@localhost:5432/openmachinecore < migrations/001_init.sql
	psql postgresql://omc:omc@localhost:5432/openmachinecore < migrations/002_workflow_engine.sql
	@echo "Migrations complete"

migrate-down:
	@echo "Rolling back database migrations..."
	psql postgresql://omc:omc@localhost:5432/openmachinecore < migrations/down/002_workflow_engine.sql
	psql postgresql://omc:omc@localhost:5432/openmachinecore < migrations/down/001_init.sql
	@echo "Rollback complete"

# Create new migration
migrate-create:
	@read -p "Enter migration name: " name; \
	timestamp=$$(date +%Y%m%d%H%M%S); \
	echo "Creating migration: $${timestamp}_$${name}.sql"; \
	touch migrations/$${timestamp}_$${name}.sql; \
	mkdir -p migrations/down; \
	touch migrations/down/$${timestamp}_$${name}.sql

# Lint code (requires golangci-lint)
lint:
	@echo "Running golangci-lint..."
	golangci-lint run --timeout=5m
	@echo "Lint complete"

# Install development tools
dev-tools: proto-install
	@echo "Installing development tools..."
	$(GOCMD) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Development tools installed"

# Help
help:
	@echo "OpenMachineCore Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  Development:"
	@echo "    make                    - Generate proto and build"
	@echo "    make proto-install      - Install protobuf tools"
	@echo "    make proto              - Generate protobuf code"
	@echo "    make proto-clean        - Clean generated proto files"
	@echo "    make build              - Build application"
	@echo "    make build-linux        - Build for Linux"
	@echo "    make run                - Run application (without build)"
	@echo "    make run-build          - Build and run application"
	@echo "    make dev-tools          - Install all development tools"
	@echo ""
	@echo "  Testing:"
	@echo "    make test               - Run tests"
	@echo "    make test-coverage      - Run tests with coverage"
	@echo "    make coverage           - Generate coverage HTML report"
	@echo "    make vet                - Run go vet"
	@echo "    make lint               - Run golangci-lint"
	@echo ""
	@echo "  Code Quality:"
	@echo "    make fmt                - Format code"
	@echo "    make tidy               - Tidy dependencies"
	@echo "    make deps               - Download dependencies"
	@echo ""
	@echo "  Cleanup:"
	@echo "    make clean              - Clean build artifacts"
	@echo "    make clean-all          - Clean everything including proto"
	@echo ""
	@echo "  Docker:"
	@echo "    make docker-build       - Build Docker image"
	@echo "    make docker-run         - Run Docker container"
	@echo "    make docker-compose-up  - Start with docker-compose"
	@echo "    make docker-compose-down - Stop docker-compose"
	@echo "    make docker-rebuild     - Rebuild docker-compose from scratch"
	@echo ""
	@echo "  Database:"
	@echo "    make migrate-up         - Run database migrations"
	@echo "    make migrate-down       - Rollback database migrations"
	@echo "    make migrate-create     - Create new migration"
	@echo ""
	@echo "  Help:"
	@echo "    make help               - Show this help"
