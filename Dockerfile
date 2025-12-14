# Multi-stage build for DB Backup Utility

# Stage 1: Build
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build binaries
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT} -w -s" \
    -a -installsuffix cgo \
    -o /bin/db-backup \
    ./cmd/cli/main.go

RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -X main.GitCommit=${GIT_COMMIT} -w -s" \
    -a -installsuffix cgo \
    -o /bin/db-backup-server \
    ./cmd/server/main.go

# Stage 2: Runtime (CLI)
FROM alpine:latest AS cli

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    mysql-client \
    postgresql-client \
    mongodb-tools \
    sqlite \
    tzdata

# Create non-root user
RUN addgroup -g 1000 dbbackup && \
    adduser -D -u 1000 -G dbbackup dbbackup

# Create directories
RUN mkdir -p /data/backups /config && \
    chown -R dbbackup:dbbackup /data /config

# Copy binary from builder
COPY --from=builder /bin/db-backup /usr/local/bin/db-backup

# Set user
USER dbbackup

# Set working directory
WORKDIR /data

# Entry point
ENTRYPOINT ["db-backup"]
CMD ["--help"]

# Stage 3: Runtime (Server)
FROM alpine:latest AS server

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    mysql-client \
    postgresql-client \
    mongodb-tools \
    sqlite \
    tzdata

# Create non-root user
RUN addgroup -g 1000 dbbackup && \
    adduser -D -u 1000 -G dbbackup dbbackup

# Create directories
RUN mkdir -p /data/backups /config && \
    chown -R dbbackup:dbbackup /data /config

# Copy binary from builder
COPY --from=builder /bin/db-backup-server /usr/local/bin/db-backup-server

# Copy default config
COPY config.yaml.example /config/config.yaml

# Set user
USER dbbackup

# Expose API port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/v1/health || exit 1

# Set working directory
WORKDIR /data

# Entry point
ENTRYPOINT ["db-backup-server"]
