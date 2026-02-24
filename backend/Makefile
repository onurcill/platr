.PHONY: dev build tidy lint test

# Download dependencies
tidy:
	go mod tidy

# Run in development mode (with live reload via `air` if installed)
dev:
	@if command -v air > /dev/null; then \
		air; \
	else \
		go run ./...; \
	fi

# Build binary
build:
	go build -o bin/grpc-inspector ./...

# Run tests
test:
	go test ./... -v

# Lint (requires golangci-lint)
lint:
	golangci-lint run ./...

# Install air for live reload
install-air:
	go install github.com/air-verse/air@latest
