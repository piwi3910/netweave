-- name: CreateTenant :exec
INSERT INTO tenants (
    id, name, description, status, quota, usage,
    contact_email, metadata, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: GetTenant :one
SELECT * FROM tenants WHERE id = $1;

-- name: UpdateTenant :exec
UPDATE tenants SET
    name = $2,
    description = $3,
    status = $4,
    quota = $5,
    usage = $6,
    contact_email = $7,
    metadata = $8,
    updated_at = $9
WHERE id = $1;

-- name: DeleteTenant :execrows
DELETE FROM tenants WHERE id = $1;

-- name: ListTenants :many
SELECT * FROM tenants ORDER BY created_at DESC;

-- name: TenantExists :one
SELECT EXISTS(SELECT 1 FROM tenants WHERE id = $1);

-- name: IncrementTenantUsage :exec
UPDATE tenants SET
    usage = jsonb_set(
        usage,
        ARRAY[$2],
        to_jsonb(COALESCE((usage->>$2)::int, 0) + 1)
    ),
    updated_at = NOW()
WHERE id = $1;

-- name: DecrementTenantUsage :exec
UPDATE tenants SET
    usage = jsonb_set(
        usage,
        ARRAY[$2],
        to_jsonb(GREATEST(COALESCE((usage->>$2)::int, 0) - 1, 0))
    ),
    updated_at = NOW()
WHERE id = $1;
