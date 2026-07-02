-- name: FindUserByID :one
SELECT id, banned, ban_reason, ban_expires
FROM users
WHERE id = $1
LIMIT 1;
