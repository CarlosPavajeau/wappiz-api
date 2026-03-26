-- name: UpdateCustomer :exec
UPDATE customers
SET name = $1
WHERE id = $2;