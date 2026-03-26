-- name: FindAppointmentStatusHistory :many
SELECT h.id,
       h.appointment_id,
       h.from_status,
       h.to_status,
       u.name as changed_by,
       h.changed_by_role,
       h.reason,
       h.created_at
FROM appointment_status_history h
         JOIN appointments a ON a.id = h.appointment_id
         LEFT JOIN users u ON u.id = h.changed_by
WHERE h.appointment_id = $1
  AND a.tenant_id = $2
ORDER BY h.created_at;