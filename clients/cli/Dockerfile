## Multi-stage Dockerfile for ProgressDB CLI Benchmark Machine
# Builds CLI binary and packages it into a minimal runtime image with keep-alive service

### Build stage
FROM golang:1.24 AS builder
WORKDIR /src

# Copy go.mod and go.sum first for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build CLI with module path
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o prgcli .

### Runtime stage
FROM debian:bookworm-slim

# Install basic tools and certificates
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN groupadd --gid 1000 bench && \
    useradd --uid 1000 --gid 1000 --create-home --home-dir /home/bench bench

# Copy CLI binary from builder
COPY --from=builder /src/prgcli /usr/local/bin/prgcli
RUN chmod +x /usr/local/bin/prgcli

# Create a simple keep-alive service to keep the machine running
RUN echo '#!/bin/bash\nwhile true; do sleep 3600; done' > /keepalive.sh && \
    chmod +x /keepalive.sh

# Set up user environment
USER bench
WORKDIR /home/bench

# Expose a port (required for Fly services, even though we don't use it)
EXPOSE 8080

# Keep the machine alive with a simple sleep loop
CMD ["/keepalive.sh"]