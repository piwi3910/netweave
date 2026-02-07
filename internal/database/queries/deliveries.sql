-- name: TrackDelivery :exec
INSERT INTO notification_deliveries (
    id, event_id, subscription_id, callback_url,
    status, attempts, max_attempts,
    last_attempt_at, next_attempt_at, last_error,
    http_status_code, response_time,
    created_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
ON CONFLICT (id) DO UPDATE SET
    status = EXCLUDED.status,
    attempts = EXCLUDED.attempts,
    last_attempt_at = EXCLUDED.last_attempt_at,
    next_attempt_at = EXCLUDED.next_attempt_at,
    last_error = EXCLUDED.last_error,
    http_status_code = EXCLUDED.http_status_code,
    response_time = EXCLUDED.response_time,
    completed_at = EXCLUDED.completed_at;

-- name: GetDelivery :one
SELECT * FROM notification_deliveries WHERE id = $1;

-- name: ListDeliveriesByEvent :many
SELECT * FROM notification_deliveries
WHERE event_id = $1
ORDER BY created_at DESC;

-- name: ListDeliveriesBySubscription :many
SELECT * FROM notification_deliveries
WHERE subscription_id = $1
ORDER BY created_at DESC;

-- name: ListFailedDeliveries :many
SELECT * FROM notification_deliveries
WHERE status = 'failed'
ORDER BY created_at DESC;
