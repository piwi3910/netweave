# Deployment Guide

This guide provides detailed instructions for deploying the netweave O2-IMS Gateway in different environments.

## Prerequisites

- Kubernetes 1.30+ cluster with access
- Go 1.25.0+ (for development)
- Docker (for building containers)
- kubectl configured
- make
- Helm 3.x+

## Deployment Options

There are three primary methods for deploying the netweave gateway:

1.  **Quick Deploy (Development)**: A simple make command to get a development environment up and running quickly.
2.  **Production Deployment (Helm)**: The recommended method for production deployments, using Helm for configuration and lifecycle management.
3.  **Production Deployment (Operator)**: For Kubernetes-native lifecycle management using a Custom Resource.

## Option 1: Quick Deploy (Development)

This method is ideal for local development and testing.

```bash
# Clone the repository
git clone https://github.com/piwi3910/netweave.git
cd netweave

# Install development tools
make install-tools

# Build and deploy to Kubernetes
make deploy-dev
```

This will build the gateway, create a Docker image, and deploy it to your currently configured Kubernetes cluster.

## Option 2: Production Deployment (Helm)

This is the recommended method for staging and production environments.

### 1. Install Prerequisites

**cert-manager**: Required for automatic TLS certificate management.

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.0/cert-manager.yaml
```

### 2. Install Redis

A Redis Sentinel cluster is required for high availability.

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install redis bitnami/redis \
  --namespace o2ims-system \
  --create-namespace \
  --set sentinel.enabled=true
```

### 3. Deploy netweave

Deploy the gateway using the Helm chart located in the `helm/netweave` directory.

```bash
helm install netweave ./helm/netweave \
  --namespace o2ims-system \
  --values helm/netweave/values-prod.yaml
```

### 4. Verify Deployment

```bash
kubectl get pods -n o2ims-system
```

You should see the gateway pods, Redis pods, and the subscription controller running.

## Option 3: Production Deployment (Operator)

This method uses a Kubernetes Operator to manage the gateway's lifecycle.

### 1. Install the Operator

```bash
kubectl apply -f deployments/operator/crd.yaml
kubectl apply -f deployments/operator/operator.yaml
```

### 2. Deploy netweave via Custom Resource

Create a `O2IMSGateway` custom resource to deploy the gateway.

```yaml
apiVersion: o2ims.oran.org/v1alpha1
kind: O2IMSGateway
metadata:
  name: netweave-production
  namespace: o2ims-system
spec:
  replicas: 3
  version: "v1.0.0"
  tls:
    enabled: true
    issuerRef:
      name: ca-issuer
      kind: ClusterIssuer
  redis:
    sentinel: true
    replicas: 3
```

Apply the custom resource:

```bash
kubectl apply -f o2imsgateway.yaml
```

### 3. Verify Deployment

```bash
kubectl get o2imsgateways -n o2ims-system
kubectl get pods -n o2ims-system
```

## Deployment Topologies

The gateway can be deployed in different topologies depending on the environment.

*   **Development**: 1 cluster, 1 pod, no Redis Sentinel, basic TLS, minimal observability.
*   **Staging**: 1 cluster, 3 pods, Redis Sentinel, mTLS, full observability.
*   **Production**: 2+ clusters, 3+ pods per cluster, Redis HA cross-cluster, mTLS with cert rotation, full observability.

## Deployment Strategies

### Rolling Update (Zero-Downtime)

This is the default strategy, providing zero-downtime deployments.

```yaml
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 1
    maxSurge: 1
```

### Blue-Green Deployment

For maximum safety, you can use a blue-green deployment strategy.

1.  Deploy the new version (`v2`) alongside the current version (`v1`).
2.  Test the `v2` deployment internally.
3.  Switch traffic to `v2` by updating the Service selector.
4.  Monitor `v2` for issues.
5.  Delete the `v1` deployment.

### Canary Deployment

For a gradual rollout, you can use a canary deployment with a service mesh like Istio.

```yaml
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: netweave-gateway
spec:
  hosts:
    - netweave-gateway
  http:
    - route:
        - destination:
            host: netweave-gateway
            subset: v1
          weight: 90
        - destination:
            host: netweave-gateway
            subset: v2
          weight: 10 # 10% of traffic to v2
```
