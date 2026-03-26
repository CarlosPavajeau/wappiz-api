-- name: FindCustomersByTenant :many
SELECT id, tenant_id, phone_number, name, is_blocked, created_at
FROM customers
WHERE tenant_id = $1
ORDER BY created_at DESC;