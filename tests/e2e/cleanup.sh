#!/bin/bash
# E2E Test Environment Cleanup Script
# Tears down the Kind cluster and cleans up test resources

set -euo pipefail

CLUSTER_NAME="${CLUSTER_NAME:-netweave-e2e}"

echo "==> Cleaning up E2E test environment"

# Kill port-forward if running
if [ -f /tmp/netweave-e2e-port-forward.pid ]; then
    PID=$(cat /tmp/netweave-e2e-port-forward.pid)
    if ps -p "${PID}" >/dev/null 2>&1; then
        echo "==> Killing port-forward (PID: ${PID})"
        kill "${PID}" 2>/dev/null || true
    fi
    rm -f /tmp/netweave-e2e-port-forward.pid
fi

# Delete Kind cluster
if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "==> Deleting Kind cluster '${CLUSTER_NAME}'"
    kind delete cluster --name "${CLUSTER_NAME}"
else
    echo "Kind cluster '${CLUSTER_NAME}' does not exist"
fi

echo "==> Cleanup complete!"
