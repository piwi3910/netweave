# Technology Stack

## Core Technologies

| Layer | Technology | Version | Purpose |
|-------|-----------|---------|---------|
| Language | Go | 1.23+ | Core implementation |
| Framework | Gin | 1.10+ | HTTP server |
| Orchestration | Kubernetes | 1.30+ | Infrastructure platform |
| TLS | Native Go + cert-manager | 1.15+ | mTLS, certificate management |
| Storage | Redis OSS | 7.4+ | State, cache, pub/sub |
| Deployment | Helm + Custom Operator | 3.x+ | Application lifecycle |
| Metrics | Prometheus | 2.54+ | Monitoring |
| Tracing | Jaeger | 1.60+ | Distributed tracing |
| Logging | Zap | 1.27+ | Structured logging |

## Key Dependencies

- github.com/gin-gonic/gin - HTTP web framework
- github.com/redis/go-redis/v9 - Redis client
- k8s.io/client-go - Kubernetes client library
- github.com/spf13/viper - Configuration management
- go.uber.org/zap - Structured logging
- github.com/prometheus/client_golang - Prometheus metrics
- github.com/google/uuid - UUID generation
- github.com/stretchr/testify - Testing framework
- github.com/alicebob/miniredis/v2 - Redis mocking for tests

## Architecture Components

- Gateway Pods: Stateless HTTP servers handling O2-IMS API requests
- Redis: State storage for subscriptions, caching, pub/sub coordination
- Subscription Controller: Watches K8s resources and sends webhook notifications
- Adapters: Backend integrations (currently Kubernetes, extensible to other platforms)
- Plugin System: Registry-based routing to multiple backend adapters
