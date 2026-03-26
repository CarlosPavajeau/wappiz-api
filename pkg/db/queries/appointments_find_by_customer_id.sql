-- name: FindAppointmentsByCustomerID :many
SELECT a.id,
       a.starts_at,
       a.ends_at,
       a.status,
       a.price_at_booking,
       r.name AS resource_name,
       s.name AS service_name
FROM appointments a
         JOIN resources r ON r.id = a.resource_id
         JOIN services s ON s.id = a.service_id
WHERE a.tenant_id = $1
  AND a.customer_id = $2
  AND a.status = 'confirmed'
  AND a.starts_at > NOW()
ORDER BY a.starts_at
LIMIT 5;