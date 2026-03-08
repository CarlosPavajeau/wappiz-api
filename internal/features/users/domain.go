package users

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	Email        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}
