-- name: FindResourceOccupiedSlots :many
SELECT starts_at, ends_at
FROM appointments
WHERE resource_id = $1
  AND starts_at >= $2
  AND ends_at <= $3
  AND status NOT IN ('cancelled', 'no_show')
ORDER BY starts_at;