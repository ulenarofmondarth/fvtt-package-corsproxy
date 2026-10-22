.PHONY: help build run dev test coverage smoke lint fmt fmt-check dist install docker docker-build docker-run \
	docker-compose-up docker-compose-down clean config

# The version stamped into the binary (`/health` and the startup log). Releases pass the tag.
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

# The coverage gate (percent of statements). Ratchet it up as coverage improves; never down.
COVERAGE_MIN ?= 86

# The platforms each release ships a binary for.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

# Default target
help:
	@echo "Available targets:"
	@echo "  make build         - Build the binary into bin/"
	@echo "  make run           - Run the server"
	@echo "  make dev           - Run with hot reload (requires air)"
	@echo "  make test          - Run the Go tests"
	@echo "  make coverage      - Run the Go tests with the coverage gate ($(COVERAGE_MIN)%)"
	@echo "  make smoke         - Start the server and run test.sh against real sites (needs network)"
	@echo "  make lint          - go vet and golangci-lint"
	@echo "  make fmt           - Format the Go code"
	@echo "  make fmt-check     - Fail if any Go file is not gofmt'd"
	@echo "  make dist          - Cross-compile the release binaries into dist/"
	@echo "  make docker        - Build and run the Docker image"
	@echo "  make install       - Download the Go dependencies"
	@echo "  make clean         - Remove build artifacts"
	@echo "  make config        - Show the configuration from the environment"

build:
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/cors-proxy .

run:
	go run .

# Development mode with hot reload (requires air)
dev:
	@command -v air > /dev/null 2>&1 || (echo "Installing air..." && go install github.com/air-verse/air@latest)
	air

test:
	go test ./...

coverage:
	go test ./... -coverprofile=coverage.out -covermode=atomic
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/ { sub(/%/, "", $$3); print $$3 }'); \
	echo "Coverage: $${total}% (minimum $(COVERAGE_MIN)%)"; \
	awk -v total="$$total" -v min="$(COVERAGE_MIN)" 'BEGIN { exit !(total >= min) }' \
		|| { echo "Coverage is below the minimum."; exit 1; }

# The end-to-end smoke test: a real server, fetching from real sites.
smoke:
	./test.sh

lint:
	go vet ./...
	golangci-lint run

fmt:
	gofmt -w .

fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then echo "Not gofmt'd:"; echo "$$unformatted"; exit 1; fi

dist:
	rm -rf dist && mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; ext=; \
		[ "$$os" = windows ] && ext=.exe; \
		echo "Building $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags="$(LDFLAGS)" \
			-o "dist/cors-proxy-$$os-$$arch$$ext" . || exit 1; \
	done
	cd dist && sha256sum cors-proxy-* > checksums.txt

install:
	go mod download
	go mod tidy

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t cors-proxy:latest .

docker-run:
	docker run -p 8080:8080 --rm cors-proxy:latest

docker: docker-build docker-run

docker-compose-up:
	docker compose up --build

docker-compose-down:
	docker compose down

clean:
	rm -rf bin/ dist/ coverage.out
	go clean

# Show current configuration
config:
	@echo "PORT: $${PORT:-8080}"
	@echo "MAX_REQUEST_SIZE: $${MAX_REQUEST_SIZE:-10485760}"
	@echo "REQUEST_TIMEOUT: $${REQUEST_TIMEOUT:-30s}"
	@echo "MAX_REDIRECTS: $${MAX_REDIRECTS:-10}"
	@echo "ALLOWED_ORIGINS: $${ALLOWED_ORIGINS:-*}"
	@echo "ALLOWED_HOSTS: $${ALLOWED_HOSTS:-}"
	@echo "BLOCKED_HOSTS: $${BLOCKED_HOSTS:-}"
	@echo "ALLOW_PRIVATE_NETWORKS: $${ALLOW_PRIVATE_NETWORKS:-false}"
	@echo "RATE_LIMIT_PER_MINUTE: $${RATE_LIMIT_PER_MINUTE:-0}"
	@echo "TRUST_PROXY_HEADERS: $${TRUST_PROXY_HEADERS:-false}"
	@echo "REQUIRE_API_KEY: $${REQUIRE_API_KEY:-false}"
	@echo "DAILY_REQUEST_LIMIT: $${DAILY_REQUEST_LIMIT:-0}"
	@echo "CACHE_TTL: $${CACHE_TTL:-0}"
	@echo "CACHE_NEGATIVE_TTL: $${CACHE_NEGATIVE_TTL:-5m}"
	@echo "CACHE_MAX_STALE: $${CACHE_MAX_STALE:-24h}"
	@echo "CACHE_MAX_BYTES: $${CACHE_MAX_BYTES:-67108864}"
	@echo "CACHE_MAX_ENTRY_BYTES: $${CACHE_MAX_ENTRY_BYTES:-2097152}"
	@echo "VERBOSE_LOGGING: $${VERBOSE_LOGGING:-false}"
