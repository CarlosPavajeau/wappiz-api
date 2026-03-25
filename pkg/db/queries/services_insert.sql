-- name: InsertService :exec
INSERT INTO services(
    id,
    tenant_id,
    name,
    description,
    duration_minutes,
    buffer_minutes,
    price,
    is_active,
    sort_order
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    true,
    $8
);