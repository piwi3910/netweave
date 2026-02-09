-- name: CreateCertificateMetadata :exec
INSERT INTO certificate_metadata (
    serial_number, user_id, tenant_id, common_name, role_name,
    status, issued_at, expires_at, renewed_from, renewed_to,
    renewal_count, last_error, retry_count, next_retry_at,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16);

-- name: GetCertificateMetadata :one
SELECT * FROM certificate_metadata WHERE serial_number = $1;

-- name: UpdateCertificateStatus :exec
UPDATE certificate_metadata SET
    status = $2,
    updated_at = NOW()
WHERE serial_number = $1;

-- name: UpdateCertificateRevoked :exec
UPDATE certificate_metadata SET
    status = 'revoked',
    revoked_at = NOW(),
    updated_at = NOW()
WHERE serial_number = $1;

-- name: UpdateCertificateRenewed :exec
UPDATE certificate_metadata SET
    status = 'renewed',
    renewed_at = NOW(),
    renewed_to = $2,
    renewal_count = renewal_count + 1,
    updated_at = NOW()
WHERE serial_number = $1;

-- name: UpdateCertificateRenewalFailed :exec
UPDATE certificate_metadata SET
    status = 'renewal_failed',
    last_error = $2,
    retry_count = retry_count + 1,
    next_retry_at = $3,
    updated_at = NOW()
WHERE serial_number = $1;

-- name: ListCertificatesByTenant :many
SELECT * FROM certificate_metadata
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: ListCertificatesByUser :many
SELECT * FROM certificate_metadata
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListCertificatesByStatus :many
SELECT * FROM certificate_metadata
WHERE status = $1
ORDER BY created_at DESC;

-- name: ListExpiringCertificates :many
SELECT * FROM certificate_metadata
WHERE status = 'active'
  AND expires_at <= $1
ORDER BY expires_at ASC;

-- name: ListRenewalFailedCertificates :many
SELECT * FROM certificate_metadata
WHERE status = 'renewal_failed'
  AND next_retry_at <= $1
ORDER BY next_retry_at ASC;

-- name: CountCertificatesByStatus :many
SELECT status, COUNT(*) AS count
FROM certificate_metadata
GROUP BY status;

-- name: DeleteCertificateMetadata :execrows
DELETE FROM certificate_metadata WHERE serial_number = $1;
