-- name: UpdateConversationSession :exec
UPDATE conversation_sessions
SET step       = $1,
    data       = $2,
    expires_at = $3,
    updated_at = NOW()
WHERE id = $4;