-- name: CountCustomerNoShows :one
SELECT no_show_count AS no_shows
FROM customers
WHERE id = $1
  AND tenant_id = $2;