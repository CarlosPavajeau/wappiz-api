-- name: DeleteConversationSession :exec
DELETE
FROM conversation_sessions
WHERE id = $1;