-- name: CreateBackend :exec
INSERT INTO backend_instances (
    id, name, category, adapter_type, description,
    config, credentials, status, status_message,
    last_health_check, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetBackend :one
SELECT * FROM backend_instances WHERE id = $1;

-- name: UpdateBackend :exec
UPDATE backend_instances SET
    name = $2,
    description = $3,
    config = $4,
    credentials = $5,
    status = $6,
    status_message = $7,
    last_health_check = $8,
    updated_at = $9
WHERE id = $1;

-- name: DeleteBackend :execrows
DELETE FROM backend_instances WHERE id = $1;

-- name: ListBackends :many
SELECT * FROM backend_instances ORDER BY created_at DESC;

-- name: ListBackendsByCategory :many
SELECT * FROM backend_instances
WHERE category = $1
ORDER BY created_at DESC;

-- name: BackendExists :one
SELECT EXISTS(SELECT 1 FROM backend_instances WHERE id = $1);

-- name: UpdateBackendStatus :exec
UPDATE backend_instances SET
    status = $2,
    status_message = $3,
    last_health_check = $4,
    updated_at = $5
WHERE id = $1;

-- name: CreateBackendLink :exec
INSERT INTO backend_links (ims_id, dms_id, created_at)
VALUES ($1, $2, $3);

-- name: DeleteBackendLink :execrows
DELETE FROM backend_links WHERE ims_id = $1 AND dms_id = $2;

-- name: ListDMSLinksByIMS :many
SELECT bi.* FROM backend_instances bi
INNER JOIN backend_links bl ON bi.id = bl.dms_id
WHERE bl.ims_id = $1
ORDER BY bi.name;

-- name: ListIMSLinksByDMS :many
SELECT bi.* FROM backend_instances bi
INNER JOIN backend_links bl ON bi.id = bl.ims_id
WHERE bl.dms_id = $1
ORDER BY bi.name;

-- name: CreateBackendAccess :exec
INSERT INTO backend_access (
    id, tenant_id, backend_id, permissions, constraints,
    granted_at, granted_by
) VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetBackendAccess :one
SELECT * FROM backend_access WHERE id = $1;

-- name: UpdateBackendAccess :exec
UPDATE backend_access SET
    permissions = $2,
    constraints = $3
WHERE id = $1;

-- name: DeleteBackendAccess :execrows
DELETE FROM backend_access WHERE id = $1;

-- name: ListBackendAccessByTenant :many
SELECT * FROM backend_access
WHERE tenant_id = $1
ORDER BY granted_at DESC;

-- name: ListBackendAccessByBackend :many
SELECT * FROM backend_access
WHERE backend_id = $1
ORDER BY granted_at DESC;

-- name: BackendAccessExists :one
SELECT EXISTS(
    SELECT 1 FROM backend_access
    WHERE tenant_id = $1 AND backend_id = $2
);
