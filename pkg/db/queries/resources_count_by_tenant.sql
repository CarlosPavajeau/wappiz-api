-- name: CountResourcesByTenant :one
SELECT tenant_id,
       COUNT(*) AS count
FROM resources
WHERE tenant_id = $1
GROUP BY tenant_id;