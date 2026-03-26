-- name: InsertAppointment :exec
INSERT INTO appointments(
    id,
    tenant_id,
    resource_id,
    service_id,
    customer_id,
    starts_at,
    ends_at,
    status,
    price_at_booking
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    'confirmed',
    $8
);