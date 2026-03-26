-- name: FindRecentlyCancelledAppointments :many
SELECT id,
       tenant_id,
       customer_id,
       resource_id,
       service_id,
       starts_at,
       ends_at,
       status,
       cancelled_at
FROM appointments
WHERE status = 'cancelled'
  AND cancelled_at IS NOT NULL
  AND cancelled_at >= NOW() - INTERVAL '10 minutes';