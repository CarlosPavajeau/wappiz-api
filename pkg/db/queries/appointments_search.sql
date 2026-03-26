-- name: SearchAppointments :many
SELECT a.id,
       a.starts_at,
       a.ends_at,
       a.status,
       a.price_at_booking,
       r.name                           AS resource_name,
       s.name                           AS service_name,
       COALESCE(c.name, c.phone_number) AS customer_name
FROM appointments a
         JOIN resources r ON r.id = a.resource_id
         JOIN services s ON s.id = a.service_id
         JOIN customers c ON c.id = a.customer_id
WHERE a.tenant_id = $1
  AND a.starts_at >= $2
  AND a.starts_at < $3
  AND a.resource_id = ANY (sqlc.slice(resource_ids))
  AND a.service_id = ANY (sqlc.slice(service_ids))
  AND a.status = ANY (sqlc.slice(status))
ORDER BY a.starts_at;