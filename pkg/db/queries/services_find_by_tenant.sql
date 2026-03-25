-- name: FindServicesByTenantID :many
SELECT id,
       tenant_id,
       name,
       description,
       duration_minutes,
       buffer_minutes,
       price,
       is_active,
       sort_order,
       created_at
FROM services
WHERE tenant_id = $1
  AND is_active = true
ORDER BY created_at;