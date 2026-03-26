-- name: InsertConversationSession :exec
INSERT INTO conversation_sessions(
    id,
    tenant_id,
    whatsapp_config_id,
    customer_id,
    step,
    data,
    expires_at
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
) ON CONFLICT (tenant_id, customer_id) DO UPDATE
    SET step       = EXCLUDED.step,
        data       = EXCLUDED.data,
        expires_at = EXCLUDED.expires_at,
        updated_at = NOW();