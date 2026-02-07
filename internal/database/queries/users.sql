-- name: CreateUser :exec
INSERT INTO users (
    id, tenant_id, subject, common_name, email,
    oauth_subject, oauth_provider, role_id, is_active,
    last_login_at, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserBySubject :one
SELECT * FROM users WHERE subject = $1 AND subject != '';

-- name: GetUserByOAuthSubject :one
SELECT * FROM users WHERE oauth_subject = $1 AND oauth_subject != '';

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 AND email != '';

-- name: UpdateUser :exec
UPDATE users SET
    tenant_id = $2,
    subject = $3,
    common_name = $4,
    email = $5,
    oauth_subject = $6,
    oauth_provider = $7,
    role_id = $8,
    is_active = $9,
    updated_at = $10
WHERE id = $1;

-- name: DeleteUser :execrows
DELETE FROM users WHERE id = $1;

-- name: ListUsersByTenant :many
SELECT * FROM users WHERE tenant_id = $1 ORDER BY created_at DESC;

-- name: UpdateLastLogin :exec
UPDATE users SET last_login_at = NOW() WHERE id = $1;

-- name: UserExists :one
SELECT EXISTS(SELECT 1 FROM users WHERE id = $1);
