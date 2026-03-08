package auth

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// RefreshToken represents a persisted refresh token used to reissue access tokens.
// Token rotation is enforced: each token is single-use and belongs to a family
// so that reuse of a revoked token triggers revocation of the entire family.
type RefreshToken struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	FamilyID  uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

func (rt *RefreshToken) IsExpired() bool {
	return time.Now().After(rt.ExpiresAt)
}

func (rt *RefreshToken) IsRevoked() bool {
	return rt.RevokedAt != nil
}

// TokenPair holds an access/refresh token pair issued after authentication.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

var (
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrRefreshTokenReuse   = errors.New("refresh token reuse detected")
	ErrRefreshTokenExpired = errors.New("refresh token expired")
)
