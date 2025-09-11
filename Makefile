.PHONY: build run clean test docker-build docker-run

# Binary output
BINARY_NAME=chat-server

# Build the application
build:
	go build -o $(BINARY_NAME) ./cmd/ts-chat

# Run the application
run: build
	./$(BINARY_NAME)

# Clean build artifacts
clean:
	rm -f $(BINARY_NAME)

# Run tests
test:
	go test -v ./internal/chat/

# Build Docker image
docker-build:
	docker build -t chat-server .

# Run Docker container
docker-run: docker-build
	docker run -p 2323:2323 chat-server

# Cross-compile for different platforms
build-all: build-linux build-macos build-windows build-arm

# Linux amd64
build-linux:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY_NAME)-linux-amd64 ./cmd/ts-chat

# macOS amd64
build-macos:
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY_NAME)-darwin-amd64 ./cmd/ts-chat

# Windows amd64
build-windows:
	GOOS=windows GOARCH=amd64 go build -o $(BINARY_NAME)-windows-amd64.exe ./cmd/ts-chat

# ARM (Raspberry Pi)
build-arm:
	GOOS=linux GOARCH=arm go build -o $(BINARY_NAME)-linux-arm ./cmd/ts-chat