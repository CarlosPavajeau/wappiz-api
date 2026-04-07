-- name: IncrementCustomerNoShows :exec
UPDATE customers
SET no_show_count = no_show_count + 1
WHERE id = $1 AND tenant_id = $2;