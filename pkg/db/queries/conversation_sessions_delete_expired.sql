-- name: DeleteExpiredConversationSessions :exec
DELETE
FROM conversation_sessions
WHERE expires_at < NOW();