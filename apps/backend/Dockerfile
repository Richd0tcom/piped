# Build stage
FROM --platform=linux/amd64 golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
# RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o piped ./cmd/main.go

RUN CGO_ENABLED=0 GOOS=linux go build -a -o piped ./cmd/main.go

# Runtime stage
FROM --platform=linux/amd64 debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    docker.io \
    && rm -rf /var/lib/apt/lists/*

RUN VERSION=$(curl -fsSL "https://api.github.com/repos/railwayapp/railpack/releases/latest" \
      | grep '"tag_name"' | cut -d'"' -f4 | sed 's/^v//') && \
    curl -fsSL "https://github.com/railwayapp/railpack/releases/download/v${VERSION}/railpack-v${VERSION}-x86_64-unknown-linux-musl.tar.gz" \
      -o /tmp/railpack.tar.gz && \
    tar -xzf /tmp/railpack.tar.gz -C /usr/local/bin && \
    chmod +x /usr/local/bin/railpack && \
    rm /tmp/railpack.tar.gz

RUN railpack --version

WORKDIR /app
COPY --from=builder /app/piped .

RUN mkdir -p /data /tmp/piped/uploads /tmp/piped/builds

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["./piped"]