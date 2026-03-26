-- name: FindAppointmentByID :one
SELECT id,
       tenant_id,
       resource_id,
       service_id,
       customer_id,
       starts_at,
       ends_at,
       status,
       price_at_booking
FROM appointments
WHERE id = $1
  AND tenant_id = $2
LIMIT 1;