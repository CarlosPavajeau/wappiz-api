-- name: UpdateService :exec
UPDATE services
SET name             = $1,
    description      = $2,
    duration_minutes = $3,
    buffer_minutes   = $4,
    price            = $5,
    sort_order       = $6
WHERE id = $7
  AND tenant_id = $8;