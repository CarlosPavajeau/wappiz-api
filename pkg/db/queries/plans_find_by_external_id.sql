-- name: FindPlanByExternalId :one
SELECT id,
       external_id,
       name,
       description,
       price,
       currency,
       "interval"
FROM plans
WHERE is_active = true
    AND external_id = $1 AND environment = $2;
