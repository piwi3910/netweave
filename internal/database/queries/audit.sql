-- name: InsertAuditEvent :exec
INSERT INTO audit_events (
    id, type, tenant_id, user_id, subject,
    resource_type, resource_id, action, details,
    client_ip, user_agent, timestamp,
    prev_hash, entry_hash
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: GetLatestAuditHash :one
SELECT entry_hash
FROM audit_events
ORDER BY timestamp DESC, id DESC
LIMIT 1;

-- name: ListAuditEventsForVerification :many
SELECT id, prev_hash, entry_hash, timestamp
FROM audit_events
ORDER BY timestamp ASC, id ASC
LIMIT $1 OFFSET $2;

-- name: ListAuditEvents :many
SELECT * FROM audit_events
WHERE (sqlc.arg(tenant_filter)::text = '' OR tenant_id = sqlc.arg(tenant_filter)::text)
ORDER BY timestamp DESC
LIMIT $1 OFFSET $2;

-- name: ListAuditEventsByType :many
SELECT * FROM audit_events
WHERE type = $1
ORDER BY timestamp DESC
LIMIT $2;

-- name: ListAuditEventsByUser :many
SELECT * FROM audit_events
WHERE user_id = $1
ORDER BY timestamp DESC
LIMIT $2;

-- name: CleanupOldAuditEvents :execrows
DELETE FROM audit_events
WHERE timestamp < NOW() - make_interval(days => $1);
