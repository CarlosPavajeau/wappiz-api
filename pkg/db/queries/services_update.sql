-- name: UpdateService :exec
UPDATE services
SET name             = $1,
    description      = $2,
    duration_minutes = $3,
    buffer_minutes   = $4,
    price            = $5,
    sort_order       = $6,
    is_active        = $7
WHERE id = $8
  AND tenant_id = $9;