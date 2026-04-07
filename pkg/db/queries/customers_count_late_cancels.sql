-- name: CountCustomerLateCancels :one
SELECT late_cancel_count AS late_cancels
FROM customers
WHERE id = $1
  AND tenant_id = $2;