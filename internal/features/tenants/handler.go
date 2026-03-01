package tenants

import (
	"appointments/internal/shared/jwt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	useCases *UseCases
}

func NewHandler(uc *UseCases) *Handler {
	return &Handler{useCases: uc}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	public := r.Group("/api/v1")
	{
		public.POST("/tenants/register", h.Register)
		public.POST("/tenants/login", h.Login)
	}

	protected := r.Group("/api/v1/tenants")
	protected.Use(jwt.AuthMiddleware())
	{
		protected.GET("/me", h.GetMe)
		protected.PUT("/settings", h.UpdateSettings)
		protected.POST("/whatsapp", h.ConnectWhatsapp)
	}
}

type registerRequest struct {
	Name     string `json:"name"     binding:"required,min=2"`
	Timezone string `json:"timezone" binding:"required"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

type loginRequest struct {
	Slug     string `json:"slug"     binding:"required"`
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type connectWhatsappRequest struct {
	WabaID             string `json:"waba_id"              binding:"required"`
	PhoneNumberID      string `json:"phone_number_id"      binding:"required"`
	DisplayPhoneNumber string `json:"display_phone_number" binding:"required"`
	AccessToken        string `json:"access_token"         binding:"required"`
}

type updateSettingsRequest struct {
	WelcomeMessage  string `json:"welcome_message"`
	BotName         string `json:"bot_name"`
	CancellationMsg string `json:"cancellation_message"`
}

func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	output, err := h.useCases.RegisterTenant(c.Request.Context(), RegisterTenantInput{
		Name:     req.Name,
		Timezone: req.Timezone,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}

	token, err := jwt.Generate(output.User.ID, output.Tenant.ID, output.User.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"tenant": gin.H{
			"id":   output.Tenant.ID,
			"name": output.Tenant.Name,
			"slug": output.Tenant.Slug,
			"plan": output.Tenant.Plan,
		},
		"token": token,
	})
}

func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, tenant, err := h.useCases.Login(c.Request.Context(), LoginInput{
		TenantSlug: req.Slug,
		Email:      req.Email,
		Password:   req.Password,
	})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, err := jwt.Generate(user.ID, tenant.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"tenant": gin.H{
			"id":   tenant.ID,
			"name": tenant.Name,
			"slug": tenant.Slug,
			"plan": tenant.Plan,
		},
		"token": token,
	})
}

func (h *Handler) GetMe(c *gin.Context) {
	tenantID := jwt.TenantIDFromContext(c)
	tenant, err := h.useCases.repo.FindByID(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tenant not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                      tenant.ID,
		"name":                    tenant.Name,
		"slug":                    tenant.Slug,
		"timezone":                tenant.Timezone,
		"plan":                    tenant.Plan,
		"appointments_this_month": tenant.AppointmentsThisMonth,
		"settings":                tenant.Settings,
	})
}

func (h *Handler) UpdateSettings(c *gin.Context) {
	var req updateSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := jwt.TenantIDFromContext(c)
	err := h.useCases.UpdateSettings(c.Request.Context(), tenantID, TenantSettings{
		WelcomeMessage:  req.WelcomeMessage,
		BotName:         req.BotName,
		CancellationMsg: req.CancellationMsg,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "settings updated"})
}

func (h *Handler) ConnectWhatsapp(c *gin.Context) {
	var req connectWhatsappRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := jwt.TenantIDFromContext(c)
	cfg, err := h.useCases.ConnectWhatsapp(c.Request.Context(), ConnectWhatsappInput{
		TenantID:           tenantID,
		WabaID:             req.WabaID,
		PhoneNumberID:      req.PhoneNumberID,
		DisplayPhoneNumber: req.DisplayPhoneNumber,
		AccessToken:        req.AccessToken,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "connection failed"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"phone_number_id":      cfg.PhoneNumberID,
		"display_phone_number": cfg.DisplayPhoneNumber,
		"is_active":            cfg.IsActive,
	})
}
