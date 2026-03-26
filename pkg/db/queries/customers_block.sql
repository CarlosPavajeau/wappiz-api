-- name: BlockCustomer :exec
UPDATE customers
SET is_blocked = true
WHERE id = $1
  AND tenant_id = $2;