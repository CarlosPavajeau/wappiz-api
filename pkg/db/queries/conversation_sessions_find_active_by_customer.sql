-- name: FindCustomerActiveConversationSession :one
SELECT id,
       tenant_id,
       whatsapp_config_id,
       customer_id,
       step,
       data,
       expires_at,
       created_at,
       updated_at
FROM conversation_sessions
WHERE tenant_id = $1
  AND customer_id = $2
  AND expires_at > NOW()
LIMIT 1;