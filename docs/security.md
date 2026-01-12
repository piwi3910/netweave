# Security Guide

This document outlines the security architecture and best practices for the netweave O2-IMS Gateway.

## Zero-Trust Principles

The gateway is designed with a zero-trust security model, which means:

1.  **Assume Breach**: Design for scenarios where the perimeter is compromised.
2.  **Verify Explicitly**: Authenticate and authorize every request.
3.  **Least Privilege**: Minimal permissions for all components.
4.  **Encrypt Everything**: mTLS for all communication.

## Authentication & Authorization

### North-Bound (SMO to Gateway)

*   **mTLS**: All communication between the SMO and the gateway is secured with mutual TLS (mTLS).
*   **Client Certificates**: The SMO must present a valid client certificate issued by a trusted Certificate Authority (CA).
*   **Authorization**: The gateway uses the Common Name (CN) from the client certificate to identify the SMO and apply authorization rules.

### South-Bound (Gateway to Kubernetes)

*   **ServiceAccount**: The gateway uses a Kubernetes ServiceAccount to communicate with the Kubernetes API server.
*   **RBAC**: The ServiceAccount is granted the least privilege required to perform its functions, using a `ClusterRole` with specific permissions.

## OpenAPI Request Validation

The gateway validates all incoming requests against the OpenAPI 3.0 specification. This provides:

*   **Input Sanitization**: Rejects malformed requests before they reach the handlers.
*   **DoS Protection**: Body size limits prevent memory exhaustion attacks.
*   **Schema Enforcement**: Ensures O2-IMS compliance at the API boundary.

## Distributed Rate Limiting

To protect against DDoS attacks and resource exhaustion, the gateway implements distributed rate limiting using a Redis-backed token bucket algorithm.

*   **Multi-Level Limits**: Per-tenant, per-endpoint, and global rate limits.
*   **Atomic Operations**: Redis Lua scripts ensure consistency across all gateway pods.
*   **Standard Headers**: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`, `Retry-After`.
*   **Graceful Degradation**: Fails open if Redis is unavailable.

## mTLS Architecture

### Certificate Hierarchy

A `cert-manager` `ClusterIssuer` is used to create a root CA, which in turn signs intermediate CAs for different purposes:

*   **Server CA**: For gateway pods, Redis, and other internal services.
*   **Client CA**: For external clients like the SMO.
*   **Webhook CA**: For the subscription controller to use when calling SMO webhooks.

### Secrets Management

*   **No Hardcoded Secrets**: All secrets are managed through Kubernetes Secrets.
*   **cert-manager**: Automates the issuance and renewal of TLS certificates.
*   **External Secrets Operator**: Can be used to sync secrets from an external secret store like HashiCorp Vault or AWS Secrets Manager.

## Network Security

### Network Policies

Kubernetes `NetworkPolicy` resources are used to restrict traffic to and from the gateway pods.

*   **Ingress**: Only allows traffic from the ingress controller.
*   **Egress**: Only allows traffic to the Kubernetes API server, Redis, and external SMO webhooks.

## Security Monitoring

*   **Audit Logging**: All API requests, authentication failures, and authorization denials are logged.
*   **Metrics**: Prometheus metrics for authentication, authorization, and TLS.
*   **Alerts**: Alerts for expiring certificates, repeated authentication failures, and other security events.
