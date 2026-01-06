# Docker Deployment Guide

Quick reference for building and running the O2-IMS Gateway with Docker.

## Building the Image

### Using Makefile (Recommended)

```bash
# Build Docker image with version tags
make docker-build

# Build with custom version
VERSION=v0.2.0 make docker-build

# Security scan the image
make docker-scan

# Build and push to registry
make docker-push
```

### Manual Build

```bash
# Build with default tags
docker build -t netweave:latest .

# Build with version information
docker build \
  --build-arg VERSION=v0.2.0 \
  --build-arg COMMIT=$(git rev-parse --short HEAD) \
  --build-arg BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S') \
  -t netweave:v0.2.0 \
  -t netweave:latest \
  .

# Build for specific platforms (multi-arch)
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t netweave:latest \
  --push \
  .
```

## Running Locally

### Standalone (Development)

```bash
# Run with default configuration
docker run -p 8080:8080 netweave:latest

# Run with environment variables
docker run -p 8080:8080 \
  -e NETWEAVE_OBSERVABILITY_LOGGING_LEVEL=debug \
  -e NETWEAVE_REDIS_ADDRESSES=redis:6379 \
  netweave:latest

# Run with custom config file
docker run -p 8080:8080 \
  -v $(pwd)/config/config.yaml:/etc/netweave/config/config.yaml \
  netweave:latest

# Run in background (detached)
docker run -d \
  --name netweave-gateway \
  -p 8080:8080 \
  netweave:latest
```

### With Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  redis:
    image: redis:7.4-alpine
    container_name: netweave-redis
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    command: redis-server --appendonly yes
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  gateway:
    image: netweave:latest
    container_name: netweave-gateway
    depends_on:
      redis:
        condition: service_healthy
    ports:
      - "8080:8080"
    environment:
      NETWEAVE_REDIS_ADDRESSES: redis:6379
      NETWEAVE_OBSERVABILITY_LOGGING_LEVEL: info
      NETWEAVE_OBSERVABILITY_LOGGING_FORMAT: json
    volumes:
      - ./config/config.yaml:/etc/netweave/config/config.yaml
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 30s
      timeout: 3s
      retries: 3
      start_period: 5s
    restart: unless-stopped

volumes:
  redis-data:
```

Run with Docker Compose:

```bash
# Start services
docker-compose up -d

# View logs
docker-compose logs -f gateway

# Stop services
docker-compose down

# Stop and remove volumes
docker-compose down -v
```

## Image Management

### Inspect the Image

```bash
# View image details
docker inspect netweave:latest

# View image layers
docker history netweave:latest

# View image size
docker images netweave
```

### Tag and Push

```bash
# Tag for different registries
docker tag netweave:latest myregistry.io/netweave:v0.2.0
docker tag netweave:latest ghcr.io/piwi3910/netweave:v0.2.0

# Push to Docker Hub
docker push docker.io/netweave:v0.2.0

# Push to GitHub Container Registry
docker push ghcr.io/piwi3910/netweave:v0.2.0

# Push to private registry
docker push myregistry.io/netweave:v0.2.0
```

### Pull from Registry

```bash
# Pull from Docker Hub
docker pull netweave:latest

# Pull from GitHub Container Registry
docker pull ghcr.io/piwi3910/netweave:v0.2.0

# Pull from private registry
docker pull myregistry.io/netweave:v0.2.0
```

## Security Scanning

### Using Trivy

```bash
# Scan for vulnerabilities (via Makefile)
make docker-scan

# Manual scan
trivy image netweave:latest

# Scan with specific severity
trivy image --severity HIGH,CRITICAL netweave:latest

# Generate JSON report
trivy image -f json -o scan-report.json netweave:latest
```

### Using Docker Scout

```bash
# Enable Docker Scout
docker scout quickview

# Analyze image
docker scout cves netweave:latest

# Compare images
docker scout compare netweave:v0.1.0 --to netweave:v0.2.0
```

### Using Snyk

```bash
# Install Snyk CLI
npm install -g snyk

# Authenticate
snyk auth

# Scan image
snyk container test netweave:latest

# Monitor image
snyk container monitor netweave:latest
```

## Testing

### Health Check

```bash
# Check if container is healthy
docker ps --filter name=netweave-gateway

# Manually test health endpoint
docker exec netweave-gateway wget -qO- http://localhost:8080/health

# Or from host
curl http://localhost:8080/health
```

### Logs

```bash
# View logs
docker logs netweave-gateway

# Follow logs
docker logs -f netweave-gateway

# Last 100 lines
docker logs --tail 100 netweave-gateway

# With timestamps
docker logs -t netweave-gateway
```

### Shell Access

```bash
# Execute shell in running container
docker exec -it netweave-gateway /bin/sh

# Run a command
docker exec netweave-gateway ps aux

# Check configuration
docker exec netweave-gateway cat /etc/netweave/config/config.yaml
```

### Metrics

```bash
# Get container stats
docker stats netweave-gateway

# Export Prometheus metrics
curl http://localhost:8080/metrics
```

## Debugging

### Common Issues

**Container exits immediately:**

```bash
# Check logs for errors
docker logs netweave-gateway

# Run with interactive shell
docker run -it --entrypoint /bin/sh netweave:latest
```

**Cannot connect to Redis:**

```bash
# Check Redis connectivity
docker exec netweave-gateway ping redis

# Check network
docker network inspect bridge
```

**Permission issues:**

```bash
# Check user/group
docker exec netweave-gateway id

# Verify file permissions
docker exec netweave-gateway ls -la /etc/netweave/config/
```

### Network Debugging

```bash
# Inspect container network
docker inspect netweave-gateway --format='{{.NetworkSettings.IPAddress}}'

# Test connectivity
docker run --rm busybox ping <container-ip>

# Check exposed ports
docker port netweave-gateway
```

## Cleanup

```bash
# Stop and remove container
docker stop netweave-gateway
docker rm netweave-gateway

# Remove image
docker rmi netweave:latest

# Remove all stopped containers
docker container prune

# Remove all unused images
docker image prune -a

# Complete cleanup (careful!)
docker system prune -a --volumes
```

## Production Best Practices

### Resource Limits

```bash
# Run with memory limit
docker run \
  --memory=512m \
  --memory-reservation=256m \
  --cpus=0.5 \
  -p 8080:8080 \
  netweave:latest
```

### Restart Policies

```bash
# Always restart
docker run -d --restart=always netweave:latest

# Restart unless stopped
docker run -d --restart=unless-stopped netweave:latest

# Restart on failure (max 3 times)
docker run -d --restart=on-failure:3 netweave:latest
```

### Health Checks

```bash
# Run with custom health check
docker run -d \
  --health-cmd="wget -qO- http://localhost:8080/health || exit 1" \
  --health-interval=30s \
  --health-timeout=3s \
  --health-retries=3 \
  --health-start-period=5s \
  netweave:latest
```

### Read-Only Filesystem

```bash
# Run with read-only root filesystem
docker run -d \
  --read-only \
  --tmpfs /tmp:rw,noexec,nosuid,size=100m \
  -p 8080:8080 \
  netweave:latest
```

## CI/CD Integration

### GitHub Actions

```yaml
name: Build and Push Docker Image

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Login to Docker Hub
        uses: docker/login-action@v3
        with:
          username: ${{ secrets.DOCKER_USERNAME }}
          password: ${{ secrets.DOCKER_PASSWORD }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: netweave

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ github.ref_name }}
            COMMIT=${{ github.sha }}
            BUILD_TIME=${{ github.event.head_commit.timestamp }}
```

## References

- [Dockerfile best practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)
- [Docker security](https://docs.docker.com/engine/security/)
- [Multi-stage builds](https://docs.docker.com/build/building/multi-stage/)
