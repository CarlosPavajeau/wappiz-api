-- name: UpdateAppointment :exec
UPDATE appointments
SET status        = $1,
    updated_at    = NOW(),
    cancelled_by  = $2,
    cancel_reason = $3,
    completed_at  = $4
WHERE id = $5;