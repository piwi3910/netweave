# Dockerfile for O2-IMS Gateway
# Multi-stage build for minimal production image

# Stage 1: Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary
# -ldflags to reduce binary size and inject version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -v \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}" \
    -o netweave \
    ./cmd/gateway

# Stage 2: Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl \
    && update-ca-certificates

# Create non-root user
RUN addgroup -g 1000 -S netweave && \
    adduser -u 1000 -S netweave -G netweave

# Create required directories with proper permissions
RUN mkdir -p /etc/netweave/config /var/lib/netweave /var/log/netweave && \
    chown -R netweave:netweave /etc/netweave /var/lib/netweave /var/log/netweave

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder --chown=netweave:netweave /build/netweave /app/netweave

# Copy example config (can be overridden via ConfigMap)
COPY --from=builder --chown=netweave:netweave /build/config/config.yaml.example /etc/netweave/config/config.yaml.example

# Switch to non-root user
USER netweave:netweave

# Expose HTTP port
EXPOSE 8080

# Health check endpoint
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Environment variables (can be overridden)
ENV NETWEAVE_SERVER_HOST=0.0.0.0 \
    NETWEAVE_SERVER_PORT=8080 \
    NETWEAVE_OBSERVABILITY_LOGGING_LEVEL=info \
    NETWEAVE_OBSERVABILITY_LOGGING_FORMAT=json \
    NETWEAVE_OBSERVABILITY_METRICS_ENABLED=true

# Labels for metadata
LABEL maintainer="O2-IMS Gateway Team" \
      org.opencontainers.image.title="O2-IMS Gateway" \
      org.opencontainers.image.description="ORAN O2-IMS compliant API gateway for Kubernetes" \
      org.opencontainers.image.vendor="netweave" \
      org.opencontainers.image.source="https://github.com/piwi3910/netweave" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"

# Run the gateway
ENTRYPOINT ["/app/netweave"]
CMD ["--config", "/etc/netweave/config/config.yaml"]
