-- name: RejectTenantActivation :exec
UPDATE tenant_whatsapp_configs
SET reject_reason     = $1,
    activation_status = 'failed',
    updated_at        = NOW()
WHERE tenant_id = $2;
