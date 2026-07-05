package middleware

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"
	"wappiz/pkg/db"
	"wappiz/pkg/jwt"
	"wappiz/pkg/logger"
	"wappiz/svc/api/openapi"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// TenantIDLookup resolves a tenant UUID from a user ID.
// It is called by WithAuthentication after token verification to populate the
// tenant_id context value.
type TenantIDLookup func(ctx context.Context, userID string) (uuid.UUID, error)

// AuthConfig holds the dependencies for WithAuthentication.
type AuthConfig struct {
	// Verifier validates bearer tokens. Required.
	Verifier jwt.Verifier[*jwt.Claims]

	// DB is used to load the authenticated user for the ban check. Required.
	DB db.DBTX

	// TenantFinder resolves the tenant for the authenticated user. Optional;
	// when nil, or when lookup fails, tenant_id is not set on the context.
	TenantFinder TenantIDLookup
}

// WithAuthentication returns middleware that validates Bearer JWTs, rejects
// banned users, and populates user_id, role, and tenant_id context values.
func WithAuthentication(cfg AuthConfig) gin.HandlerFunc {
	if cfg.Verifier == nil {
		panic("middleware: AuthConfig.Verifier is required")
	}
	if cfg.DB == nil {
		panic("middleware: AuthConfig.DB is required")
	}

	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if len(header) < 8 || header[:7] != "Bearer " {
			c.AbortWithStatusJSON(http.StatusUnauthorized, openapi.UnauthorizedErrorResponse{
				Meta: openapi.Meta{
					RequestId: c.GetString("request_id"),
				},
				Error: openapi.BaseError{
					Title:  "Missing or invalid Authorization header.",
					Type:   "unauthorized",
					Detail: "JWT header is missing or is malformed.",
					Status: http.StatusUnauthorized,
				},
			})
			return
		}

		claims, err := cfg.Verifier.Verify(c.Request.Context(), header[7:])
		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, openapi.UnauthorizedErrorResponse{
					Meta: openapi.Meta{
						RequestId: c.GetString("request_id"),
					},
					Error: openapi.BaseError{
						Title:  "Token expired",
						Type:   "unauthorized",
						Detail: "Authentication token is expired. Please obtain a new token and try again.",
						Status: http.StatusUnauthorized,
					},
				})
				return
			}

			// Log the specific failure but keep the response generic: verification
			// errors can reveal key material state (e.g. which kids exist).
			logger.Warn("[auth] token verification failed", "err", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, openapi.UnauthorizedErrorResponse{
				Meta: openapi.Meta{
					RequestId: c.GetString("request_id"),
				},
				Error: openapi.BaseError{
					Title:  "Token invalid",
					Type:   "unauthorized",
					Detail: "Authentication token is invalid.",
					Status: http.StatusUnauthorized,
				},
			})
			return
		}

		user, err := db.Query.FindUserByID(c.Request.Context(), cfg.DB, claims.UserID)
		if errors.Is(err, sql.ErrNoRows) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, openapi.UnauthorizedErrorResponse{
				Meta: openapi.Meta{
					RequestId: c.GetString("request_id"),
				},
				Error: openapi.BaseError{
					Title:  "User not found",
					Type:   "unauthorized",
					Detail: "The account associated with this token no longer exists.",
					Status: http.StatusUnauthorized,
				},
			})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, openapi.InternalServerErrorResponse{
				Meta: openapi.Meta{
					RequestId: c.GetString("request_id"),
				},
				Error: openapi.BaseError{
					Title:  "User verification failed.",
					Type:   "internal_server_error",
					Detail: "Could not verify the account associated with this token. Please try again.",
					Status: http.StatusInternalServerError,
				},
			})
			return
		}
		if isBanned(user, time.Now()) {
			detail := "Your account has been banned. Contact support."
			if user.BanReason.Valid && user.BanReason.String != "" {
				detail = fmt.Sprintf("Your account has been banned: %s", user.BanReason.String)
			}
			c.AbortWithStatusJSON(http.StatusForbidden, openapi.ForbiddenErrorResponse{
				Meta: openapi.Meta{
					RequestId: c.GetString("request_id"),
				},
				Error: openapi.BaseError{
					Title:  "Account banned",
					Type:   "forbidden",
					Detail: detail,
					Status: http.StatusForbidden,
				},
			})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)

		if cfg.TenantFinder != nil {
			tenantID, err := cfg.TenantFinder(c.Request.Context(), claims.UserID)
			if err == nil {
				c.Set("tenant_id", tenantID)
			}
		}

		c.Next()
	}
}

// isBanned reports whether the user has an active ban. A ban with a
// ban_expires timestamp in the past has lapsed and no longer blocks access;
// a NULL ban_expires means the ban is permanent.
func isBanned(user db.FindUserByIDRow, now time.Time) bool {
	if !user.Banned.Valid || !user.Banned.Bool {
		return false
	}
	return !user.BanExpires.Valid || user.BanExpires.Time.After(now)
}

func TenantIDFromContext(c *gin.Context) uuid.UUID {
	return c.MustGet("tenant_id").(uuid.UUID)
}

// TenantIDFromContextOK returns the tenant UUID and whether it was present.
// Use this when the tenant may not exist yet (e.g. first-time registration).
func TenantIDFromContextOK(c *gin.Context) (uuid.UUID, bool) {
	v, exists := c.Get("tenant_id")
	if !exists {
		return uuid.UUID{}, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok
}

func UserIDFromContext(c *gin.Context) string {
	return c.MustGet("user_id").(string)
}
