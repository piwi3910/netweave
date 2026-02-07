-- name: CreateRole :exec
INSERT INTO roles (
    id, name, type, description, permissions, tenant_id,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: GetRole :one
SELECT * FROM roles WHERE id = $1;

-- name: GetRoleByName :one
SELECT * FROM roles WHERE name = $1;

-- name: UpdateRole :exec
UPDATE roles SET
    name = $2,
    type = $3,
    description = $4,
    permissions = $5,
    tenant_id = $6,
    updated_at = $7
WHERE id = $1;

-- name: DeleteRole :execrows
DELETE FROM roles WHERE id = $1;

-- name: ListRoles :many
SELECT * FROM roles ORDER BY created_at DESC;

-- name: ListRolesByTenant :many
SELECT * FROM roles
WHERE tenant_id = '' OR tenant_id = $1
ORDER BY created_at DESC;

-- name: RoleExists :one
SELECT EXISTS(SELECT 1 FROM roles WHERE id = $1);

-- name: RoleExistsByName :one
SELECT EXISTS(SELECT 1 FROM roles WHERE name = $1);
