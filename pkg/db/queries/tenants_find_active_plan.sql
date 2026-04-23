-- name: FindActivePlanByTenant :one
SELECT p.id,
       p.external_id,
       p.environment,
       p.features
FROM subscriptions ts
         JOIN plans p ON p.id = ts.plan_id
WHERE ts.tenant_id = $1
  AND ts.status IN ('active', 'trialing')
  AND ts.environment = $2
LIMIT 1;