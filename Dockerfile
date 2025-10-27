# Build stage
FROM golang:1.25.2-bookworm AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN go build -o claude-tools-mcp ./cmd/claude-tools-mcp

# Runtime stage - use pre-built runtime image from GHCR
# To build locally without GHCR: docker build -f Dockerfile.runtime -t claude-tools-runtime .
FROM ghcr.io/brwse/claude-tools-mcp-runtime:latest

# Switch to root to copy binary
USER root

# Copy the binary from builder
COPY --from=builder /build/claude-tools-mcp /usr/local/bin/claude-tools-mcp

# Expose HTTP port (Smithery will set PORT env var)
EXPOSE 8080

# Switch back to non-root user
USER claude

# Start the MCP server - use PORT env var if set, otherwise default to 8080
CMD ["/bin/sh", "-c", "/usr/local/bin/claude-tools-mcp --addr 0.0.0.0:${PORT:-8080}"]
