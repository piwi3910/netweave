#!/bin/bash
# E2E Test Environment Setup Script
# Sets up a Kind cluster with netweave for end-to-end testing

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

CLUSTER_NAME="${CLUSTER_NAME:-netweave-e2e}"
NAMESPACE="${NAMESPACE:-netweave-e2e}"

echo "==> Setting up E2E test environment"
echo "Cluster: ${CLUSTER_NAME}"
echo "Namespace: ${NAMESPACE}"

# Check prerequisites
command -v kind >/dev/null 2>&1 || { echo "Error: kind not found. Install from: https://kind.sigs.k8s.io/"; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "Error: kubectl not found"; exit 1; }
command -v helm >/dev/null 2>&1 || { echo "Error: helm not found"; exit 1; }

# Create Kind cluster if it doesn't exist
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "==> Kind cluster '${CLUSTER_NAME}' already exists"
else
    echo "==> Creating Kind cluster '${CLUSTER_NAME}'"
    kind create cluster --name "${CLUSTER_NAME}" --wait 5m
fi

# Set kubectl context
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# Create namespace
echo "==> Creating namespace '${NAMESPACE}'"
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

# Install Redis (required for gateway)
echo "==> Installing Redis"
helm repo add bitnami https://charts.bitnami.com/bitnami 2>/dev/null || true
helm repo update

if helm list -n "${NAMESPACE}" | grep -q "redis"; then
    echo "Redis already installed"
else
    # Deploy Redis for E2E testing
    # Note: auth.enabled=false is ONLY acceptable for isolated E2E test environments.
    # Production deployments must enable authentication and TLS.
    helm install redis bitnami/redis \
        --namespace "${NAMESPACE}" \
        --set auth.enabled=false \
        --set master.persistence.enabled=false \
        --set replica.replicaCount=0 \
        --wait \
        --timeout 5m
fi

# Build and load gateway image into Kind
echo "==> Building gateway image"
cd "${PROJECT_ROOT}"
docker build -t netweave/gateway:e2e-test .

echo "==> Loading image into Kind cluster"
kind load docker-image netweave/gateway:e2e-test --name "${CLUSTER_NAME}"

# Deploy gateway using Helm
echo "==> Deploying netweave gateway"
helm upgrade --install netweave "${PROJECT_ROOT}/helm/netweave" \
    --namespace "${NAMESPACE}" \
    --set image.repository=netweave/gateway \
    --set image.tag=e2e-test \
    --set image.pullPolicy=Never \
    --set redis.enabled=false \
    --set config.redis.addresses="{redis-master.${NAMESPACE}.svc.cluster.local:6379}" \
    --set config.server.tls.enabled=false \
    --set config.rateLimit.enabled=false \
    --set config.multi_tenancy.enabled=false \
    --wait \
    --timeout 5m

# Wait for gateway to be ready
echo "==> Waiting for gateway to be ready"
kubectl wait --for=condition=ready pod \
    -l app.kubernetes.io/name=netweave \
    -n "${NAMESPACE}" \
    --timeout=5m

# Setup port-forward for testing
echo "==> Setting up port-forward (background)"
kubectl port-forward -n "${NAMESPACE}" \
    svc/netweave 8080:8080 \
    >/dev/null 2>&1 &

PORT_FORWARD_PID=$!
echo "${PORT_FORWARD_PID}" > /tmp/netweave-e2e-port-forward.pid

# Wait for port-forward to be ready
echo "Waiting for port-forward to be ready..."
MAX_RETRIES=30
RETRY_COUNT=0
while ! curl -s -f http://localhost:8080/healthz >/dev/null 2>&1; do
    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ ${RETRY_COUNT} -ge ${MAX_RETRIES} ]; then
        echo "ERROR: Port-forward failed to become ready after ${MAX_RETRIES} attempts"
        kubectl port-forward -n "${NAMESPACE}" svc/netweave 8080:8080 &
        exit 1
    fi
    echo "Waiting for gateway to be ready (attempt ${RETRY_COUNT}/${MAX_RETRIES})..."
    sleep 1
done
echo "Port-forward is ready!"

echo "==> E2E test environment ready!"
echo "Gateway URL: http://localhost:8080"
echo "Namespace: ${NAMESPACE}"
echo "Port-forward PID: ${PORT_FORWARD_PID}"
echo ""
echo "Run tests with: make test-e2e"
echo "Cleanup with: make test-e2e-cleanup"
