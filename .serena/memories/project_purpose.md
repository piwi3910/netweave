# Project Purpose

**netweave** is a production-grade O-RAN O2-IMS compliant API gateway that enables Service Management and Orchestration (SMO) systems to manage Kubernetes-based infrastructure through standardized O2-IMS APIs.

## Key Features

- O2-IMS Compliant: Full implementation of O-RAN O2 Infrastructure Management Services specification
- Kubernetes Native: Translates O2-IMS requests to native Kubernetes API operations
- Enterprise Multi-Tenancy: Built-in support for multiple SMO systems with strict resource isolation
- Comprehensive RBAC: Fine-grained role-based access control
- Multi-Cluster Ready: Deploy across single or multiple Kubernetes clusters with Redis-based state synchronization
- High Availability: Stateless gateway pods with automatic failover (99.9% uptime)
- Production Security: mTLS everywhere, zero-trust networking, tenant isolation
- Real-Time Notifications: Webhook-based subscriptions for infrastructure change events
- Extensible Architecture: Plugin-based adapter system for future backend integrations
- Enterprise Observability: Prometheus metrics, Jaeger tracing, structured logging

## Use Cases

1. Telecom RAN Management: Manage O-Cloud infrastructure for 5G RAN workloads via standard O2-IMS APIs
2. Multi-SMO Environments: Single gateway supporting multiple SMO systems with isolated resources
3. Multi-Vendor Disaggregation: Abstract vendor-specific APIs behind O2-IMS standard interface
4. Cloud-Native Infrastructure: Leverage Kubernetes for infrastructure lifecycle management
5. Subscription-Based Monitoring: Real-time notifications of infrastructure changes to SMO systems
