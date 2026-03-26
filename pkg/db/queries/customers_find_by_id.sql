-- name: FindCustomerByID :one
SELECT id, tenant_id, phone_number, name, is_blocked, created_at
FROM customers
WHERE id = $1
LIMIT 1;