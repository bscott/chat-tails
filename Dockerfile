FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o chat-server ./cmd/ts-chat

# Create final lightweight image
FROM alpine:latest

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/chat-server .

# Set environment variables
ENV PORT=2323 \
    ROOM_NAME="Chat Room" \
    MAX_USERS=10 \
    TS_AUTHKEY=""

# Expose the port
EXPOSE ${PORT}

# Run the application with a custom entrypoint script
COPY --from=builder /app/install/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]