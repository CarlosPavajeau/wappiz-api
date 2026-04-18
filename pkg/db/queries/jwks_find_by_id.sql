-- name: FindJWKByID :one
SELECT id, public_key
FROM jwks
WHERE id = $1
  AND (expires_at IS NULL OR expires_at > NOW())
LIMIT 1;
