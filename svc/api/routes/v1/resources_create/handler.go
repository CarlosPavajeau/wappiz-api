package resources_create

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"wappiz/pkg/db"
	"wappiz/pkg/jwt"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	freePlanLimit = 1
)

type Request struct {
	Name      string `json:"name"      binding:"required,min=2"`
	Type      string `json:"type"      binding:"required"`
	AvatarURL string `json:"avatarUrl"`
}

type Handler struct {
	DB          db.Database
	Environment string
}

func (h *Handler) Method() string { return http.MethodPost }
func (h *Handler) Path() string   { return "/v1/resources" }

func (h *Handler) Handle(c *gin.Context) {
	var req Request
	if err := c.ShouldBindJSON(&req); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := jwt.TenantIDFromContext(c)
	ctx := c.Request.Context()

	limited, err := h.isResourceLimitReached(ctx, tenantID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if limited {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "resource limit reached for your plan"})
		return
	}

	if err := db.Query.InsertResource(ctx, h.DB.Primary(), db.InsertResourceParams{
		ID:        uuid.New(),
		TenantID:  tenantID,
		Name:      req.Name,
		Type:      req.Type,
		AvatarUrl: sql.NullString{String: req.AvatarURL, Valid: req.AvatarURL != ""},
		SortOrder: 1,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusCreated)
}

func (h *Handler) isResourceLimitReached(ctx context.Context, tenantID uuid.UUID) (bool, error) {
	plan, err := db.Query.FindActivePlanByTenant(ctx, h.DB.Primary(), db.FindActivePlanByTenantParams{
		TenantID:    tenantID,
		Environment: h.Environment,
	})

	var limit int64 = freePlanLimit

	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		// No active plan — apply free plan limit.
	} else {
		features, err := db.UnmarshalNullableJSONTo[db.PlanFeatures](plan.Features)
		if err != nil {
			return false, err
		}

		if features.MaxResources == nil {
			return false, nil
		}

		limit = int64(*features.MaxResources)
	}

	rc, err := db.Query.CountResourcesByTenant(ctx, h.DB.Primary(), tenantID)
	if err != nil {
		return false, err
	}

	return rc.Count >= limit, nil
}
