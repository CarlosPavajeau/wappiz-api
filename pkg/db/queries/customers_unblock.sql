-- name: UnblockCustomer :exec
UPDATE customers
SET is_blocked = false
WHERE id = $1
  AND tenant_id = $2;