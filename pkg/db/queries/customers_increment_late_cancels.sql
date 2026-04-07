-- name: IncrementCustomerLateCancels :exec
UPDATE customers
SET late_cancel_count = late_cancel_count + 1
WHERE id = $1 AND tenant_id = $2;