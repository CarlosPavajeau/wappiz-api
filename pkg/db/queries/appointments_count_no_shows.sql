-- name: CountCustomerNoShows :one
SELECT COUNT(*) as no_shows
FROM appointments
WHERE tenant_id = $1
  AND customer_id = $2
  AND status = 'no_show';