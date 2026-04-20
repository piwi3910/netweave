# Breaking Changes

This document tracks breaking changes in the O2-IMS Gateway implementation.

## Unify Prometheus Metric Namespace to `netweave_*` (2026-04-20)

Resolves #490 (H17: metric naming unification) and #497 (I6: bound label cardinality).

### Summary

All Prometheus metrics emitted by the gateway now use the canonical
`Namespace: "netweave"` with a meaningful `Subsystem` label. Previously the
codebase emitted metrics under **four different** schemes (`netweave_*`,
`o2ims_*` via Namespace, flat `o2ims_*` via Name, and unprefixed
`api_request_duration_seconds`). Operators can now query every series with a
single `{__name__=~"netweave_.*"}` filter.

In the same change, label cardinality on notification delivery metrics has
been bounded: raw `subscription_id` values are hashed to a 16-bit bucket, and
`callback_url` is reduced to its host portion.

### Breaking Change Details

**Metric name renames** (non-exhaustive — every `o2ims_*` series is affected):

| Old series name                                 | New series name                                    |
| ----------------------------------------------- | -------------------------------------------------- |
| `o2ims_adapter_operations_total`                | `netweave_adapter_operations_total`                |
| `o2ims_adapter_operation_duration_seconds`      | `netweave_adapter_operation_duration_seconds`      |
| `o2ims_adapter_backend_latency_seconds`         | `netweave_adapter_backend_latency_seconds`         |
| `o2ims_adapter_health_check_status`             | `netweave_adapter_health_check_status`             |
| `o2ims_events_generated_total`                  | `netweave_events_generated_total`                  |
| `o2ims_events_queue_depth`                      | `netweave_events_queue_depth`                      |
| `o2ims_notifications_delivered_total`           | `netweave_notifications_delivered_total`           |
| `o2ims_notifications_circuit_breaker_state`     | `netweave_notifications_circuit_breaker_state`    |
| `o2ims_certificates_issuances_total`            | `netweave_certificates_issuances_total`            |
| `o2ims_webhook_deliveries_total`                | `netweave_webhook_deliveries_total`                |
| `o2ims_webhook_latency_seconds`                 | `netweave_webhook_latency_seconds`                 |
| `o2ims_webhook_dlq_total`                       | `netweave_webhook_dlq_total`                       |
| `o2ims_active_webhook_workers`                  | `netweave_webhook_active_workers`                  |
| `o2ims_event_stream_length`                     | `netweave_events_stream_length`                    |
| `o2ims_subscription_events_processed_total`     | `netweave_controller_events_processed_total`       |
| `o2ims_subscription_events_queued_total`        | `netweave_controller_events_queued_total`          |
| `o2ims_active_subscriptions`                    | `netweave_controller_active_subscriptions`         |
| `o2ims_informer_sync_duration_seconds`          | `netweave_controller_informer_sync_duration_seconds` |
| `o2ims_resource_rate_limit_hits_total`          | `netweave_ratelimit_resource_hits_total`           |
| `o2ims_resource_rate_limit_fail_open_total`     | `netweave_ratelimit_resource_fail_open_total`      |
| `o2ims_smo_api_request_duration_seconds`        | `netweave_smo_api_request_duration_seconds`        |
| `o2ims_smo_workflow_executions_total`           | `netweave_smo_workflow_executions_total`           |
| `o2ims_http_requests_total`                     | `netweave_http_requests_total`                     |
| `o2ims_http_request_duration_seconds`           | `netweave_http_request_duration_seconds`           |
| `o2ims_redis_operations_total`                  | `netweave_redis_operations_total`                  |
| `o2ims_k8s_operations_total`                    | `netweave_k8s_operations_total`                    |
| `o2ims_batch_operations_total`                  | `netweave_batch_operations_total`                  |

**Label changes (cardinality bound):**

| Metric                                         | Old label(s)                         | New label(s)                     |
| ---------------------------------------------- | ------------------------------------ | -------------------------------- |
| `netweave_notifications_delivered_total`       | `status`, `subscription_id`          | `status`, `subscription_bucket` |
| `netweave_notifications_delivery_duration_seconds` | `status`, `subscription_id`     | `status`                         |
| `netweave_notifications_attempts`              | `status`, `subscription_id`          | `status`                         |
| `netweave_notifications_response_time_seconds` | `subscription_id`, `http_status`     | `http_status`                    |
| `netweave_notifications_circuit_breaker_state` | `callback_url`                       | `callback_host`                  |
| `netweave_webhook_deliveries_total`            | `subscription_id`, `status`          | `subscription_bucket`, `status` |
| `netweave_webhook_latency_seconds`             | `subscription_id`                    | `subscription_bucket`            |
| `netweave_webhook_retries_total`               | `subscription_id`, `attempt`         | `subscription_bucket`, `attempt`|
| `netweave_webhook_dlq_total`                   | `subscription_id`                    | `subscription_bucket`            |
| `netweave_controller_events_queued_total`      | `subscription_id`, `resource_type`   | `subscription_bucket`, `resource_type` |

`subscription_bucket` is a deterministic 4-char lowercase hex value (SHA-256
of the subscription ID, first 16 bits), bounding cardinality to 65536 series
max per metric regardless of how many subscriptions exist.

`callback_host` is the host-and-port portion of the callback URL; unparseable
URLs fall back to a stable 8-char hash prefix (`hash:<xxxxxxxx>`).

### Impact

**Affected consumers:**
- Prometheus recording rules and alert expressions matching `o2ims_*`.
- Grafana dashboards with `o2ims_*` queries.
- Downstream tooling scraping `/metrics` and matching by name or label.
- Any operator tooling grouping by `subscription_id` or `callback_url`.

### Migration

1. Update PromQL in your recording rules, alerts, and dashboards:
   `o2ims_` → `netweave_`. The canonical in-repo dashboard
   (`deployments/monitoring/grafana-dashboard-adapters.json`) and alert rules
   (`deployments/monitoring/prometheus-alerts-adapters.yaml`) have been
   updated to the new names.
2. If you group by `subscription_id` to identify a specific subscription,
   switch to grouping by `subscription_bucket` (you can still correlate
   individual subscriptions back to buckets offline via
   `sha256(id)[:4] & 0xffff`, but the metrics will no longer expose raw IDs
   for DoS-hardening and privacy reasons — see #497).
3. If you group by `callback_url`, switch to `callback_host`. Path tokens
   (which may have embedded tenant or customer identifiers) are no longer
   exposed.

There is no Prometheus-native rename: the old series names will simply stop
being emitted after upgrade and new series names will start. Plan a
maintenance window and update all consumers atomically with the deployment.

## PR #194: Resource ID Format Change (2026-01-12)

### Summary
Resource IDs have been simplified from the complex `res-{type}-{uuid}` format to plain RFC 4122 compliant UUIDs.

### Breaking Change Details

**Old Format:**
```
res-compute-node-standard-a1b2c3d4-e5f6-7890-abcd-1234567890ab
```

**New Format:**
```
a1b2c3d4-e5f6-7890-abcd-1234567890ab
```

### Impact

**Affected Components:**
- POST /resources endpoint (auto-generated IDs)
- GET /resources/:id endpoint (ID lookup)
- All webhook notifications containing resource data
- External systems storing or referencing resource IDs

**Systems That May Break:**
1. **External Monitoring/Logging Systems**: Any system parsing resource IDs expecting the `res-{type}-` prefix
2. **SMO Integration**: Service Management & Orchestration systems with hardcoded ID format expectations
3. **Automation Scripts**: Scripts using regex patterns matching `res-*` format
4. **Database Queries**: Systems filtering by ID prefix (e.g., `WHERE resourceId LIKE 'res-%'`)

### Rationale

The old format was over-engineered:
- Resource type is already captured in the `resourceTypeId` field (redundant prefix)
- 50+ character IDs were unnecessarily long
- Added complexity without meaningful benefit
- Violated project principle: "avoid over-engineering"

### Migration Strategy

#### For New Deployments
No migration needed - use the new format from the start.

#### For Existing Deployments

**Option 1: Clean Slate (Recommended for Development/Test)**
1. Delete all existing resources
2. Recreate resources with new UUID format
3. Update external system integrations to remove ID format assumptions

**Option 2: Dual Format Support (For Production)**
1. Implement ID format detection in client code:
   ```go
   func isOldFormat(id string) bool {
       return strings.HasPrefix(id, "res-")
   }
   ```
2. Update external systems to handle both formats during transition
3. Plan migration window to recreate resources with new format
4. Remove old format support after migration complete

**Option 3: Backward Compatibility Shim (If Required)**
*Note: This approach is NOT recommended as it defeats the simplification purpose*
1. Add middleware to accept old format IDs
2. Map old IDs to new UUIDs in a translation table
3. Maintain translation layer indefinitely (technical debt)

### Verification Checklist

Before deploying this change:

- [ ] Audit all external systems consuming resource IDs
- [ ] Review webhook notification consumers
- [ ] Check automation scripts for ID format dependencies
- [ ] Verify logging/monitoring dashboards don't rely on ID prefixes
- [ ] Update API client libraries to use UUID validation
- [ ] Plan communication to dependent teams
- [ ] Schedule migration window if needed

### Testing

**Verify New Format:**
```bash
# Create a resource
curl -X POST http://localhost:8080/o2ims/v1/resources \
  -H "Content-Type: application/json" \
  -d '{"resourceTypeId":"compute-node","resourcePoolId":"pool-123"}'

# Response should contain UUID format:
# "resourceId": "a1b2c3d4-e5f6-7890-abcd-1234567890ab"
```

**UUID Validation:**
```go
import "github.com/google/uuid"

// Validate resource ID is proper UUID
if _, err := uuid.Parse(resourceID); err != nil {
    // Invalid UUID format
}
```

### Related Changes & ID Format Rationale

**Why Only Resources Use Plain UUIDs:**

The ID format choice for each O2-IMS entity type is intentional and based on specific use cases:

1. **Resources: Plain UUID** (`a1b2c3d4-e5f6-7890-abcd-1234567890ab`)
   - **Rationale**: Resource type is already captured in `resourceTypeId` field (redundant prefix)
   - **Benefit**: Shortest, simplest format; avoids over-engineering
   - **Use Case**: Resources are referenced by UUID in queries; type is always known from context

2. **Subscriptions: Prefixed UUID** (`sub-{uuid}`)
   - **Rationale**: Helps identify subscription IDs in logs and troubleshooting
   - **Benefit**: Distinguishes subscriptions from resources/pools when IDs appear in mixed contexts
   - **Use Case**: Subscriptions often logged separately; prefix aids debugging

3. **Resource Pools: Human-Readable + UUID** (`pool-{sanitized-name}-{uuid}`)
   - **Rationale**: Operations teams benefit from recognizable pool names in dashboards
   - **Benefit**: Human-readable names make monitoring and troubleshooting easier
   - **Use Case**: Pool IDs frequently appear in monitoring dashboards and alerts
   - **Example**: `pool-gpu-production-a1b2c3d4` is more actionable than plain UUID in alerts

**Design Principle:**
- Use **simplest format that serves the use case**
- Add complexity (prefixes, names) only when it provides **operational value**
- Resources don't need prefixes because type information is redundant
- Subscriptions and pools benefit from additional context in logs/dashboards

### Documentation Updates

- [x] docs/api-mapping.md - Updated all examples (5 instances)
- [x] README.md - Updated webhook notification example
- [x] BREAKING_CHANGES.md - This document
- [x] PR #194 description - Breaking change clearly marked

### Contact

For questions about this breaking change:
- GitHub Issue: #162
- Pull Request: #194
- Project CLAUDE.md: See "Avoid over-engineering" principle

### Rollback Plan

If this change causes critical issues:

1. **Immediate Rollback:**
   ```bash
   git revert d59db60  # Revert ID simplification commit
   git revert 58df9d2  # Revert test updates commit
   ```

2. **Restore Old Format:**
   - Revert changes in `internal/server/routes.go:1127`
   - Restore `sanitizeResourceTypeID()` function
   - Revert documentation changes

3. **Alternative: Feature Flag:**
   ```go
   // Add to config
   type Config struct {
       UseLegacyResourceIDs bool `yaml:"use_legacy_resource_ids"`
   }

   // In routes.go
   if s.config.UseLegacyResourceIDs {
       req.ResourceID = "res-" + sanitizeResourceTypeID(req.ResourceTypeID) + "-" + uuid.New().String()
   } else {
       req.ResourceID = uuid.New().String()
   }
   ```

---

**Last Updated:** 2026-01-12
**Status:** Implemented in PR #194
**Severity:** HIGH (affects API contract)
