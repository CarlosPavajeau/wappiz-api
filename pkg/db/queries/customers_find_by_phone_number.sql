-- name: FindCustomerByPhoneNumber :one
SELECT id, tenant_id, phone_number, name, is_blocked, created_at
FROM customers
WHERE tenant_id = $1
  AND phone_number = $2
LIMIT 1;