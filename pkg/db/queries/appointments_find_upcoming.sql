-- name: FindUpcomingAppointments :many
SELECT id, tenant_id, customer_id, resource_id, service_id, starts_at, ends_at
FROM appointments
WHERE status = 'confirmed'
  AND (
    (reminder_24h_sent_at IS NULL AND starts_at BETWEEN NOW() + INTERVAL '23 hours' AND NOW() + INTERVAL '25 hours')
        OR
    (reminder_1h_sent_at IS NULL AND starts_at BETWEEN NOW() + INTERVAL '50 minutes' AND NOW() + INTERVAL '70 minutes')
    );