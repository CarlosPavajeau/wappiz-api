package admin

import (
	"time"

	"github.com/google/uuid"
)

type ActivationStatus string

const (
	ActivationPending    ActivationStatus = "pending"
	ActivationInProgress ActivationStatus = "in_progress"
	ActivationActive     ActivationStatus = "active"
	ActivationFailed     ActivationStatus = "failed"
)

type Activation struct {
	TenantID     uuid.UUID
	TenantName   string
	ContactEmail string
	Notes        string
	Status       ActivationStatus
	RequestedAt  time.Time
}
