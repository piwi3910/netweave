# Breaking Changes

This document tracks breaking changes in the O2-IMS Gateway implementation.

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
