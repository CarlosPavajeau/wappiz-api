-- name: HasCustomerOverlap :one
SELECT EXISTS (
    SELECT 1
    FROM appointments a
    WHERE a.tenant_id = $1
      AND a.customer_id = $2
      AND a.status NOT IN ('cancelled', 'no_show')
  AND a.starts_at < sqlc.arg(ends_at)
  AND a.ends_at > sqlc.arg(starts_at)
) AS has_overlap;
