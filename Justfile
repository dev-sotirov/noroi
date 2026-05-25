# Noroi — Justfile (development only)

# List available recipes
default:
    @just --list

# Build the binary
build:
    go build -o dist/noroi ./cmd/server

# Build stripped binary
build-prod:
    CGO_ENABLED=0 go build -ldflags="-s -w" -o dist/noroi ./cmd/server

# Run the app locally
run:
    go run ./cmd/server/main.go

# Run all tests
test:
    go test ./...

# Run tests with race detector
test-race:
    go test -race ./...

# Run tests with coverage
test-cover:
    go test -cover ./...

# Run benchmarks
bench:
    go test -bench=. -benchmem ./...

# Format code
fmt:
    go fmt ./...

# Vet code
lint:
    go vet ./...

# Tidy dependencies
tidy:
    go mod tidy

# Build Docker image
docker-build:
    docker build -t noroi:latest .

# Start full observability stack (from deploy/)
up:
    docker compose -f deploy/docker-compose.yml up --build

# Stop full observability stack
down:
    docker compose -f deploy/docker-compose.yml down

# Clean build artifacts
clean:
    rm -f noroi
