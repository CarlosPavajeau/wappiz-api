-- name: FindUnattendedAppointments :many
SELECT id,
       tenant_id,
       customer_id,
       resource_id,
       service_id,
       starts_at,
       ends_at,
       status
FROM appointments
WHERE status = 'confirmed'
  AND starts_at <= NOW() - INTERVAL '30 minutes';