-- name: FindCustomerIncidents :many
SELECT ape.id,
       ape.event_type,
       ape.occurred_at,
       ape.appointment_id,
       a.starts_at,
       s.name AS service_name,
       r.name AS resource_name
FROM appointment_penalty_events ape
         JOIN appointments a ON a.id = ape.appointment_id
         JOIN services s ON s.id = a.service_id
         JOIN resources r ON r.id = a.resource_id
WHERE ape.customer_id = $1
  AND ape.tenant_id = $2
ORDER BY ape.occurred_at DESC
LIMIT 5;