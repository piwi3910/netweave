# Development Environment Setup

This guide provides step-by-step instructions for setting up a complete netweave development environment on macOS, Linux, and Windows.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation Steps](#installation-steps)
- [Configuration](#configuration)
- [Verification](#verification)
- [IDE Setup](#ide-setup)
- [Troubleshooting](#troubleshooting)

## Prerequisites

### System Requirements

- **OS:** macOS 12+, Linux (Ubuntu 20.04+, RHEL 8+), Windows 10+ with WSL2
- **Memory:** 8GB RAM minimum, 16GB recommended
- **Disk:** 20GB free space
- **Network:** Internet access for downloading dependencies

### Required Tools

| Tool | Min Version | Recommended | Installation |
|------|-------------|-------------|--------------|
| **Go** | 1.25.0 | Latest | [go.dev/dl](https://go.dev/dl/) |
| **Docker** | 20.10+ | Latest | [docker.com](https://www.docker.com/get-started) |
| **kubectl** | 1.30+ | 1.31+ | [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |
| **Helm** | 3.0+ | 3.16+ | [helm.sh](https://helm.sh/docs/intro/install/) |
| **Git** | 2.40+ | Latest | [git-scm.com](https://git-scm.com/downloads) |
| **make** | 3.81+ | Latest | Pre-installed on most systems |

### Optional Tools

| Tool | Purpose |
|------|---------|
| **kind** | Local Kubernetes cluster for testing |
| **minikube** | Alternative local K8s cluster |
| **k9s** | Kubernetes CLI UI |
| **Delve** | Go debugger |
| **Postman/Insomnia** | API testing |
| **Redis CLI** | Redis debugging |

---

## Installation Steps

### 1. Install Go

#### macOS (using Homebrew)

```bash
# Install latest Go
brew install go

# Verify installation
go version  # Should be 1.25.0 or higher
```

#### Linux

```bash
# Download and install
wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz

# Add to PATH (add to ~/.bashrc or ~/.zshrc)
export PATH=$PATH:/usr/local/go/bin
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin

# Reload shell
source ~/.bashrc

# Verify
go version
```

#### Windows (WSL2)

Follow Linux instructions above, or download installer from [go.dev/dl](https://go.dev/dl/).

### 2. Install Docker

#### macOS

```bash
# Install Docker Desktop
brew install --cask docker

# Start Docker Desktop from Applications folder
open -a Docker

# Verify
docker version
docker ps
```

#### Linux

```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
newgrp docker

# Verify
docker version
docker ps
```

#### Windows

1. Install [Docker Desktop for Windows](https://www.docker.com/products/docker-desktop)
2. Enable WSL2 integration
3. Restart computer
4. Verify: `docker version`

### 3. Install Kubernetes Tools

#### kubectl

```bash
# macOS
brew install kubectl

# Linux
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Verify
kubectl version --client
```

#### Helm

```bash
# macOS
brew install helm

# Linux
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Verify
helm version
```

#### kind (for local testing)

```bash
# macOS/Linux
go install sigs.k8s.io/kind@latest

# Create local cluster
kind create cluster --name netweave-dev

# Verify
kubectl cluster-info --context kind-netweave-dev
```

### 4. Clone Repository

```bash
# Clone via HTTPS
git clone https://github.com/piwi3910/netweave.git
cd netweave

# OR clone via SSH (if you have SSH keys set up)
git clone git@github.com:piwi3910/netweave.git
cd netweave
```

### 5. Install Development Tools

```bash
# Install all required development tools
make install-tools

# This installs:
# - golangci-lint (v1.56+)
# - gosec (security scanner)
# - govulncheck (vulnerability scanner)
# - mockgen (mock generator)
# - go-licenses (license checker)
# - And other tools
```

**What gets installed:**

| Tool | Purpose |
|------|---------|
| **golangci-lint** | Runs 50+ linters |
| **gosec** | Security vulnerability scanner |
| **govulncheck** | Go vulnerability checker |
| **mockgen** | Generate mocks from interfaces |
| **go-licenses** | Check dependency licenses |

### 6. Install Git Hooks

```bash
# Install pre-commit hooks
make install-hooks

# This installs hooks that automatically:
# - Format code before commit
# - Run linters
# - Run security scans
# - Prevent committing secrets
```

### 7. Download Dependencies

```bash
# Download Go module dependencies
make deps-download

# Verify dependencies
make deps-verify
```

---

## Configuration

### Configure Git

#### Set up GPG signing (Required for commits)

```bash
# Generate GPG key (if you don't have one)
gpg --full-generate-key
# Choose RSA and RSA, 4096 bits, no expiration

# List your keys
gpg --list-secret-keys --keyid-format LONG

# Output looks like:
# sec   rsa4096/ABCD1234ABCD1234 2026-01-12 [SC]
#       FEDCBA9876543210FEDCBA9876543210FEDCBA98
# uid                 [ultimate] Your Name <your.email@example.com>

# Configure git to use your key (replace with your key ID)
git config --global user.signingkey ABCD1234ABCD1234
git config --global commit.gpgsign true

# Export public key to add to GitHub
gpg --armor --export ABCD1234ABCD1234
# Copy output and add to GitHub: Settings → SSH and GPG keys → New GPG key
```

#### Set up git identity

```bash
# Set your name and email
git config --global user.name "Your Name"
git config --global user.email "your.email@example.com"

# Verify configuration
git config --list | grep user
```

### Configure Go Environment

```bash
# Set Go environment variables
export GOPATH=$HOME/go
export GOBIN=$GOPATH/bin
export PATH=$PATH:$GOBIN

# Enable Go modules (should be default in Go 1.16+)
export GO111MODULE=on

# Set private module proxy (if needed)
# export GOPRIVATE=github.com/yourorg/*

# Add to shell profile (~/.bashrc, ~/.zshrc, etc.)
echo 'export GOPATH=$HOME/go' >> ~/.zshrc
echo 'export GOBIN=$GOPATH/bin' >> ~/.zshrc
echo 'export PATH=$PATH:$GOBIN' >> ~/.zshrc
source ~/.zshrc
```

### Configure Local Kubernetes

#### Option 1: kind (Recommended for development)

```bash
# Create cluster with port forwarding for gateway
cat <<EOF | kind create cluster --name netweave-dev --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080
    hostPort: 8080
    protocol: TCP
  - containerPort: 30443
    hostPort: 8443
    protocol: TCP
EOF

# Verify cluster
kubectl cluster-info --context kind-netweave-dev
kubectl get nodes
```

#### Option 2: minikube

```bash
# Start minikube
minikube start --cpus=4 --memory=8192 --driver=docker

# Enable required addons
minikube addons enable ingress
minikube addons enable metrics-server

# Verify
kubectl get nodes
```

### Configure Redis

#### Option 1: Docker (Recommended for development)

```bash
# Start Redis container
docker run -d \
  --name netweave-redis \
  -p 6379:6379 \
  redis:7.4-alpine

# Verify
docker ps | grep redis
redis-cli ping  # Should return PONG
```

#### Option 2: Local installation

```bash
# macOS
brew install redis
brew services start redis

# Linux (Ubuntu/Debian)
sudo apt-get install redis-server
sudo systemctl start redis
sudo systemctl enable redis

# Verify
redis-cli ping  # Should return PONG
```

### Create Development Configuration

```bash
# Copy development config template
cp config/config.dev.yaml config/config.local.yaml

# Edit with your settings
vi config/config.local.yaml
```

**config.local.yaml example:**

```yaml
server:
  port: 8080
  tls:
    enabled: false  # Disable TLS for local dev

redis:
  address: "localhost:6379"
  password: ""
  sentinel:
    enabled: false

kubernetes:
  kubeconfig: ""  # Uses default kubeconfig
  context: "kind-netweave-dev"

observability:
  logging:
    level: debug
    format: console
    development: true
```

---

## Verification

### Verify Setup

```bash
# Run complete verification
make verify-setup

# This checks:
# - Go version
# - Docker availability
# - Kubernetes connectivity
# - Redis connectivity
# - Required tools installed
# - Git configuration
```

**Expected output:**

```
✓ Go version: 1.25.0
✓ Docker: Running
✓ Kubernetes: kind-netweave-dev reachable
✓ Redis: localhost:6379 responding
✓ golangci-lint: v1.56.2
✓ gosec: v2.19.0
✓ Git: Signing enabled
✓ All checks passed!
```

### Build and Test

```bash
# Format code
make fmt

# Run linters
make lint

# Run unit tests
make test

# Build binary
make build

# Run locally
make run-dev
```

### Test API Endpoint

```bash
# In another terminal, test the API
curl http://localhost:8080/o2ims/v1/deploymentManagers

# Expected response (will vary based on your K8s cluster):
{
  "items": [
    {
      "deploymentManagerId": "kind-netweave-dev",
      "name": "kind-netweave-dev",
      "description": "Kubernetes deployment manager",
      "oCloudId": "kind-netweave-dev"
    }
  ]
}
```

---

## IDE Setup

### Visual Studio Code

#### Install Extensions

```bash
# Via command line
code --install-extension golang.go
code --install-extension ms-kubernetes-tools.vscode-kubernetes-tools
code --install-extension redhat.vscode-yaml
code --install-extension eamodio.gitlens
```

#### Configure Settings

Create `.vscode/settings.json`:

```json
{
  "go.useLanguageServer": true,
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "go.formatTool": "goimports",
  "go.testFlags": ["-v", "-race"],
  "go.coverOnSave": true,
  "go.coverageDecorator": {
    "type": "gutter"
  },
  "files.eol": "\n",
  "editor.formatOnSave": true,
  "editor.codeActionsOnSave": {
    "source.organizeImports": true
  }
}
```

Create `.vscode/launch.json` for debugging:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch Gateway",
      "type": "go",
      "request": "launch",
      "mode": "debug",
      "program": "${workspaceFolder}/cmd/gateway",
      "args": [
        "--config=${workspaceFolder}/config/config.dev.yaml"
      ],
      "env": {
        "NETWEAVE_LOG_LEVEL": "debug"
      }
    }
  ]
}
```

### GoLand / IntelliJ IDEA

1. **Open Project:** File → Open → Select netweave directory
2. **Configure Go SDK:** Preferences → Go → GOROOT → Select Go 1.25+
3. **Enable Go Modules:** Preferences → Go → Go Modules → Enable
4. **Configure File Watchers:**
   - Preferences → Tools → File Watchers
   - Add: goimports, golangci-lint
5. **Configure Run Configurations:**
   - Run → Edit Configurations → Add Go Build
   - Program: `cmd/gateway`
   - Working directory: Project root
   - Environment: `NETWEAVE_LOG_LEVEL=debug`

---

## Troubleshooting

### Common Issues

#### Go Version Mismatch

**Problem:** `go: module requires Go 1.25.0`

**Solution:**
```bash
# Update Go to 1.25.0+
go version  # Check current version

# macOS
brew upgrade go

# Linux
# Download and install latest from go.dev/dl
```

#### Docker Not Running

**Problem:** `Cannot connect to Docker daemon`

**Solution:**
```bash
# macOS
open -a Docker  # Start Docker Desktop

# Linux
sudo systemctl start docker
sudo systemctl status docker
```

#### Kubernetes Cluster Unreachable

**Problem:** `The connection to the server localhost:8080 was refused`

**Solution:**
```bash
# Check kubeconfig
kubectl config view
kubectl config current-context

# kind cluster
kind get clusters
kubectl cluster-info --context kind-netweave-dev

# Restart cluster if needed
kind delete cluster --name netweave-dev
kind create cluster --name netweave-dev
```

#### Redis Connection Failed

**Problem:** `dial tcp [::1]:6379: connect: connection refused`

**Solution:**
```bash
# Check if Redis is running
docker ps | grep redis
redis-cli ping

# Start Redis container
docker run -d --name netweave-redis -p 6379:6379 redis:7.4-alpine

# Or restart Redis service
# macOS
brew services restart redis

# Linux
sudo systemctl restart redis
```

#### Linter Failures

**Problem:** `golangci-lint: command not found`

**Solution:**
```bash
# Reinstall tools
make install-tools

# Add GOBIN to PATH
export PATH=$PATH:$(go env GOPATH)/bin
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.zshrc
source ~/.zshrc
```

#### Module Dependency Issues

**Problem:** `go: module X requires Go Y`

**Solution:**
```bash
# Clean module cache
go clean -modcache

# Re-download dependencies
go mod download
go mod tidy

# Verify
make deps-verify
```

#### Build Failures

**Problem:** `undefined: X` or import errors

**Solution:**
```bash
# Clean build cache
go clean -cache

# Rebuild
go mod download
make build

# If still failing, check Go version
go version  # Must be 1.25.0+
```

### Getting Help

If you encounter issues not covered here:

1. **Check logs:**
   ```bash
   # Gateway logs
   ./bin/gateway 2>&1 | tee gateway.log

   # Docker logs
   docker logs netweave-redis
   ```

2. **Search existing issues:**
   - [GitHub Issues](https://github.com/piwi3910/netweave/issues)

3. **Ask for help:**
   - [GitHub Discussions](https://github.com/piwi3910/netweave/discussions)

4. **Provide details:**
   - OS and version
   - Go version (`go version`)
   - Docker version (`docker version`)
   - Kubernetes version (`kubectl version`)
   - Error messages and logs
   - Steps to reproduce

---

## Next Steps

✅ Development environment is now set up!

**Continue with:**

1. **[Testing Guidelines →](testing.md)** - Learn our testing philosophy
2. **[Contributing Guide →](contributing.md)** - Understand the contribution workflow
3. **[Architecture Decisions →](decisions.md)** - Review past design choices
4. **[CLAUDE.md](../../CLAUDE.md)** - Read Go-specific development standards

---

**Stuck?** Ask in [GitHub Discussions](https://github.com/piwi3910/netweave/discussions) - we're here to help!
