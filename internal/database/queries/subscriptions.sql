-- name: CreateSubscription :exec
INSERT INTO subscriptions (
    id, tenant_id, callback, consumer_subscription_id,
    filter_resource_pool_id, filter_resource_type_id, filter_resource_id,
    created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetSubscription :one
SELECT * FROM subscriptions WHERE id = $1;

-- name: UpdateSubscription :exec
UPDATE subscriptions SET
    callback = $2,
    consumer_subscription_id = $3,
    filter_resource_pool_id = $4,
    filter_resource_type_id = $5,
    filter_resource_id = $6,
    updated_at = $7
WHERE id = $1;

-- name: DeleteSubscription :execrows
DELETE FROM subscriptions WHERE id = $1;

-- name: ListSubscriptions :many
SELECT * FROM subscriptions ORDER BY created_at DESC;

-- name: ListSubscriptionsByResourcePool :many
SELECT * FROM subscriptions
WHERE filter_resource_pool_id = $1
ORDER BY created_at DESC;

-- name: ListSubscriptionsByResourceType :many
SELECT * FROM subscriptions
WHERE filter_resource_type_id = $1
ORDER BY created_at DESC;

-- name: ListSubscriptionsByTenant :many
SELECT * FROM subscriptions
WHERE tenant_id = $1
ORDER BY created_at DESC;

-- name: SubscriptionExists :one
SELECT EXISTS(SELECT 1 FROM subscriptions WHERE id = $1);
