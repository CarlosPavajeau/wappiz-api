-- name: InsertAppointmentStatusHistory :exec
INSERT INTO appointment_status_history(
    id,
    appointment_id,
    from_status,
    to_status,
    changed_by,
    changed_by_role,
    reason,
    created_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7,
    NOW()
);