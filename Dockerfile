FROM golang:1.24-alpine AS builder

WORKDIR /app

# Build arguments for version info
ARG GIT_TAG=dev
ARG GIT_COMMIT=unknown

# Copy go.mod and go.sum
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with version info
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-X main.Version=${GIT_TAG} -X main.Commit=${GIT_COMMIT}" \
    -o chat-server ./cmd/chat-tails

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