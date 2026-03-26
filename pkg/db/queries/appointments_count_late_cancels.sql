-- name: CountCustomerLateCancels :one
SELECT COUNT(*) as late_cancels
FROM appointments
WHERE status = 'cancelled'
  AND tenant_id = $1
  AND customer_id = $2
  AND cancelled_at IS NOT NULL
  AND EXTRACT(EPOCH FROM (starts_at - cancelled_at)) / 3600 < @late_hours::int;