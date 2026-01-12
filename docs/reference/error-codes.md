# Error Code Reference

Comprehensive reference for HTTP status codes, application errors, and troubleshooting guidance for netweave.

## Table of Contents

- [HTTP Status Codes](#http-status-codes)
- [Application Error Codes](#application-error-codes)
- [Adapter Error Codes](#adapter-error-codes)
- [Storage Error Codes](#storage-error-codes)
- [Common Issues](#common-issues)
- [Debugging Strategies](#debugging-strategies)

---

## HTTP Status Codes

netweave follows RFC 7807 (Problem Details for HTTP APIs) for error responses.

### Success Codes (2xx)

#### 200 OK
**Meaning:** Request succeeded, resource returned in response body

**Common for:**
- GET requests (list/get resources)
- Updates that return modified resource

**Example:**
```bash
GET /o2ims/v1/resourcePools
→ 200 OK
```

#### 201 Created
**Meaning:** Resource successfully created

**Common for:**
- POST requests creating new resources
- Response includes Location header with new resource URI

**Example:**
```bash
POST /o2ims/v1/subscriptions
→ 201 Created
Location: /o2ims/v1/subscriptions/550e8400-e29b-41d4-a716-446655440000
```

#### 204 No Content
**Meaning:** Request succeeded, no content to return

**Common for:**
- DELETE requests
- Updates that don't return modified resource

**Example:**
```bash
DELETE /o2ims/v1/subscriptions/sub-123
→ 204 No Content
```

### Client Error Codes (4xx)

#### 400 Bad Request
**Meaning:** Invalid request syntax or parameters

**Common causes:**
- Invalid JSON syntax
- Missing required fields
- Invalid field values
- Schema validation failure

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/validation-error",
  "title": "Request Validation Failed",
  "status": 400,
  "detail": "Invalid subscription callback URL",
  "instance": "/o2ims/v1/subscriptions",
  "errors": [
    {
      "field": "callback",
      "message": "Must be a valid HTTPS URL"
    }
  ]
}
```

**Solutions:**
- Validate request body against OpenAPI schema
- Check required fields are present
- Verify field value formats
- Check API documentation for correct syntax

#### 401 Unauthorized
**Meaning:** Authentication required or failed

**Common causes:**
- Missing authentication credentials
- Invalid or expired token
- Malformed Authorization header

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/authentication-failed",
  "title": "Authentication Required",
  "status": 401,
  "detail": "Valid authentication credentials required",
  "instance": "/o2ims/v1/resourcePools"
}
```

**Solutions:**
- Provide valid client certificate (mTLS)
- Check certificate hasn't expired
- Verify certificate is trusted by gateway
- Check client certificate CN matches expected format

#### 403 Forbidden
**Meaning:** Authenticated but not authorized for this operation

**Common causes:**
- Insufficient RBAC permissions
- Tenant isolation violation
- Resource access denied

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/authorization-failed",
  "title": "Access Denied",
  "status": 403,
  "detail": "Insufficient permissions to access resource pool",
  "instance": "/o2ims/v1/resourcePools/pool-123"
}
```

**Solutions:**
- Verify user has required role
- Check tenant context is correct
- Review RBAC policy configuration
- Request elevated permissions if needed

#### 404 Not Found
**Meaning:** Resource doesn't exist

**Common causes:**
- Incorrect resource ID
- Resource was deleted
- Wrong API endpoint
- Typo in URL

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/not-found",
  "title": "Resource Not Found",
  "status": 404,
  "detail": "Subscription with ID 'sub-123' not found",
  "instance": "/o2ims/v1/subscriptions/sub-123"
}
```

**Solutions:**
- Verify resource ID is correct
- List resources to find correct ID
- Check resource wasn't deleted
- Verify endpoint URL is correct

#### 409 Conflict
**Meaning:** Request conflicts with current resource state

**Common causes:**
- Duplicate resource ID
- Concurrent modification
- Resource state conflict
- Version mismatch

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/conflict",
  "title": "Resource Conflict",
  "status": 409,
  "detail": "Subscription with ID 'sub-123' already exists",
  "instance": "/o2ims/v1/subscriptions"
}
```

**Solutions:**
- Use different resource ID (or omit for auto-generation)
- Retry operation with updated data
- Delete existing resource first if replacement intended
- Check for concurrent updates

#### 422 Unprocessable Entity
**Meaning:** Syntactically valid but semantically incorrect

**Common causes:**
- Business logic violation
- Invalid state transition
- Constraint violation
- Reference to non-existent resource

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/validation-error",
  "title": "Semantic Validation Failed",
  "status": 422,
  "detail": "Cannot subscribe to non-existent resource pool",
  "instance": "/o2ims/v1/subscriptions",
  "errors": [
    {
      "field": "filter.resourcePoolId",
      "message": "Resource pool 'pool-999' does not exist"
    }
  ]
}
```

**Solutions:**
- Verify referenced resources exist
- Check business rule constraints
- Review relationship dependencies
- Validate state transitions

#### 429 Too Many Requests
**Meaning:** Rate limit exceeded

**Common causes:**
- Too many requests in short time
- Exceeded per-tenant quota
- Burst limit reached

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/rate-limit",
  "title": "Rate Limit Exceeded",
  "status": 429,
  "detail": "Rate limit of 1000 req/min exceeded for tenant",
  "instance": "/o2ims/v1/resourcePools",
  "retryAfter": 45
}
```

**Response headers:**
```
X-RateLimit-Limit: 1000
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1641024045
Retry-After: 45
```

**Solutions:**
- Implement exponential backoff retry
- Reduce request rate
- Request quota increase if legitimate need
- Batch operations where possible

### Server Error Codes (5xx)

#### 500 Internal Server Error
**Meaning:** Unexpected server error

**Common causes:**
- Unhandled exception
- Bug in code
- Resource exhaustion
- Configuration error

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/internal-error",
  "title": "Internal Server Error",
  "status": 500,
  "detail": "An unexpected error occurred processing your request",
  "instance": "/o2ims/v1/subscriptions",
  "traceId": "1234567890abcdef"
}
```

**Solutions:**
- Check gateway logs for details
- Report bug with trace ID
- Retry request (may be transient)
- Check system resource availability

#### 502 Bad Gateway
**Meaning:** Invalid response from upstream backend

**Common causes:**
- Backend adapter error
- Kubernetes API timeout
- Redis connection failed
- Backend system unavailable

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/backend-error",
  "title": "Backend Gateway Error",
  "status": 502,
  "detail": "Kubernetes API returned invalid response",
  "instance": "/o2ims/v1/resourcePools"
}
```

**Solutions:**
- Check backend system status
- Verify network connectivity
- Check Kubernetes API server health
- Review backend adapter logs

#### 503 Service Unavailable
**Meaning:** Service temporarily unable to handle request

**Common causes:**
- System maintenance
- Overloaded system
- Redis unavailable
- Starting up or shutting down

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/unavailable",
  "title": "Service Unavailable",
  "status": 503,
  "detail": "Gateway is starting up, please retry",
  "instance": "/o2ims/v1/resourcePools",
  "retryAfter": 30
}
```

**Solutions:**
- Retry with exponential backoff
- Check system status/health endpoints
- Wait for maintenance window to complete
- Verify Redis is running

#### 504 Gateway Timeout
**Meaning:** Backend operation timed out

**Common causes:**
- Long-running operation
- Backend system slow/unresponsive
- Network latency
- Resource contention

**Example response:**
```json
{
  "type": "https://netweave.example.com/errors/timeout",
  "title": "Gateway Timeout",
  "status": 504,
  "detail": "Kubernetes API request timed out after 30s",
  "instance": "/o2ims/v1/resources"
}
```

**Solutions:**
- Retry request
- Check backend performance
- Increase timeout if appropriate
- Optimize query (pagination, filtering)

---

## Application Error Codes

### Subscription Errors

| Code | Title | Description | Solution |
|------|-------|-------------|----------|
| `SUB001` | Subscription Not Found | Subscription ID doesn't exist | Verify ID, list subscriptions |
| `SUB002` | Subscription Exists | Duplicate subscription ID | Use different ID or omit for auto-gen |
| `SUB003` | Invalid Callback URL | Callback isn't valid HTTPS URL | Use valid HTTPS URL |
| `SUB004` | Webhook Delivery Failed | Unable to deliver notification | Check callback endpoint, retry |
| `SUB005` | Invalid Filter | Filter expression invalid | Review filter syntax |

### Resource Pool Errors

| Code | Title | Description | Solution |
|------|-------|-------------|----------|
| `POOL001` | Resource Pool Not Found | Pool ID doesn't exist | Verify ID, list pools |
| `POOL002` | Invalid Pool Configuration | Pool config violates constraints | Review config requirements |
| `POOL003` | Pool Creation Failed | Backend failed to create pool | Check backend logs, retry |
| `POOL004` | Pool Deletion Failed | Pool has active resources | Delete resources first |

### Resource Errors

| Code | Title | Description | Solution |
|------|-------|-------------|----------|
| `RES001` | Resource Not Found | Resource ID doesn't exist | Verify ID, list resources |
| `RES002` | Invalid Resource State | Operation invalid for current state | Check resource status first |
| `RES003` | Resource Creation Failed | Backend failed to create resource | Check backend logs, retry |
| `RES004` | Resource In Use | Resource has active workloads | Stop workloads first |

---

## Adapter Error Codes

### Kubernetes Adapter

| Error | Description | Solution |
|-------|-------------|----------|
| `KUBE_API_ERROR` | Kubernetes API call failed | Check K8s API server health, logs |
| `KUBE_UNAUTHORIZED` | Invalid kubeconfig credentials | Verify kubeconfig, service account |
| `KUBE_NOT_FOUND` | Resource not found in cluster | Verify resource exists in K8s |
| `KUBE_CONFLICT` | Resource already exists | Delete existing or use update |
| `KUBE_TIMEOUT` | Kubernetes operation timeout | Check cluster performance, increase timeout |

### OpenStack Adapter

| Error | Description | Solution |
|-------|-------------|----------|
| `OS_AUTH_FAILED` | OpenStack authentication failed | Check credentials, Keystone endpoint |
| `OS_QUOTA_EXCEEDED` | Project quota exceeded | Request quota increase |
| `OS_NOT_FOUND` | Resource not found in OpenStack | Verify resource exists |
| `OS_API_ERROR` | Nova/Neutron API error | Check OpenStack service status |

### Cloud Provider Adapters (AWS/Azure/GCP)

| Error | Description | Solution |
|-------|-------------|----------|
| `CLOUD_AUTH_FAILED` | Cloud provider auth failed | Check credentials, IAM permissions |
| `CLOUD_QUOTA_EXCEEDED` | Account quota exceeded | Request quota increase |
| `CLOUD_REGION_ERROR` | Invalid or unavailable region | Check region name, availability |
| `CLOUD_API_ERROR` | Cloud API call failed | Check service status, retry |

---

## Storage Error Codes

### Redis Errors

| Error | Description | Solution |
|-------|-------------|----------|
| `REDIS_CONN_FAILED` | Cannot connect to Redis | Check Redis is running, network |
| `REDIS_AUTH_FAILED` | Redis authentication failed | Verify password |
| `REDIS_TIMEOUT` | Operation timed out | Check Redis performance, increase timeout |
| `REDIS_OOM` | Out of memory | Increase Redis memory, check TTL |

---

## Common Issues

### Issue: "Connection refused" errors

**Symptoms:**
```
Error: dial tcp [::1]:6379: connect: connection refused
```

**Causes:**
- Redis not running
- Wrong Redis address/port
- Network connectivity issue

**Solutions:**
```bash
# Check Redis is running
docker ps | grep redis
redis-cli ping

# Start Redis if stopped
docker start netweave-redis

# Verify config
grep redis config/config.dev.yaml
```

### Issue: "Certificate verification failed"

**Symptoms:**
```
Error: x509: certificate signed by unknown authority
```

**Causes:**
- Invalid CA certificate
- Certificate expired
- Certificate CN mismatch

**Solutions:**
```bash
# Check certificate validity
openssl x509 -in client.crt -text -noout

# Verify CA trust
openssl verify -CAfile ca.crt client.crt

# Regenerate certificates if needed
make generate-certs
```

### Issue: "Resource not found" but resource exists

**Symptoms:**
- GET returns 404 but kubectl shows resource

**Causes:**
- Wrong namespace context
- RBAC permissions issue
- Cache not refreshed

**Solutions:**
```bash
# Check namespace
kubectl get nodes -A

# Verify RBAC
kubectl auth can-i get nodes

# Force cache refresh
# (restart gateway or clear Redis cache)
```

### Issue: "Rate limit exceeded" unexpectedly

**Symptoms:**
```
Error: 429 Too Many Requests
```

**Causes:**
- Legitimate high traffic
- Misconfigured rate limits
- Retry storm

**Solutions:**
```bash
# Check current limits
grep rateLimit config/config.prod.yaml

# Monitor rate limit metrics
curl http://gateway:8080/metrics | grep rate_limit

# Adjust if appropriate (in config)
rateLimit:
  perTenant:
    requestsPerMinute: 2000
```

---

## Debugging Strategies

### 1. Check Logs

```bash
# Gateway logs
kubectl logs -n o2ims-system deployment/netweave-gateway -f

# Filter for errors
kubectl logs -n o2ims-system deployment/netweave-gateway | grep ERROR

# Controller logs
kubectl logs -n o2ims-system deployment/netweave-controller -f
```

### 2. Verify Configuration

```bash
# Check config loaded correctly
kubectl get configmap -n o2ims-system netweave-config -o yaml

# Verify environment variables
kubectl describe pod -n o2ims-system <pod-name> | grep Environment -A 20

# Test configuration locally
./bin/gateway --config=config/config.dev.yaml --validate
```

### 3. Check Backend Systems

```bash
# Kubernetes API
kubectl cluster-info
kubectl get nodes

# Redis
redis-cli -h <redis-host> ping
redis-cli -h <redis-host> info

# Adapter health
curl http://gateway:8080/health/adapters
```

### 4. Trace Requests

```bash
# Enable debug logging
export NETWEAVE_LOG_LEVEL=debug

# Use trace ID from error response
grep "traceId:1234567890abcdef" /var/log/netweave/*.log

# Query Jaeger for distributed trace
open http://jaeger:16686/trace/1234567890abcdef
```

### 5. Test Isolated Components

```bash
# Test subscription directly
curl -X POST http://localhost:8080/o2ims/v1/subscriptions \
  -H "Content-Type: application/json" \
  -d '{"callback": "https://webhook.site/test"}'

# Test backend adapter
kubectl get nodes -o json

# Test Redis
redis-cli -h localhost get subscription:sub-123
```

---

## Related Documentation

- [Troubleshooting Guide](../operations/troubleshooting.md)
- [Operations Runbooks](../operations/runbooks.md)
- [API Reference](../api/README.md)
- [Glossary](glossary.md)

---

**For issues not covered here, please check [GitHub Issues](https://github.com/piwi3910/netweave/issues) or [open a new issue](https://github.com/piwi3910/netweave/issues/new).**
