-- name: Mark24hAppointmentReminderSent :exec
UPDATE appointments
SET reminder_24h_sent_at = COALESCE(reminder_24h_sent_at, $1),
    reminder_1h_sent_at  = COALESCE(reminder_1h_sent_at, $2)
WHERE id = $3;