-- name: InsertCustomer :exec
INSERT INTO customers (id, tenant_id, phone_number)
VALUES ($1, $2, $3)
ON CONFLICT (tenant_id, phone_number) DO NOTHING;