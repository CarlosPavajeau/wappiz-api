package onboarding

import (
	"context"
	"fmt"

	"appointments/internal/features/resources"
	"appointments/internal/features/services"
	"appointments/internal/features/tenants"
	"appointments/internal/platform/mailer"

	"github.com/google/uuid"
)

// TenantService defines the tenant operations needed by onboarding.
type TenantService interface {
	FindByID(ctx context.Context, id uuid.UUID) (*tenants.Tenant, error)
	CreateWhatsappConfigPending(ctx context.Context, input tenants.CreateWhatsappConfigPendingInput) error
}

type UseCases struct {
	repo          Repository
	tenantService TenantService
	resourceRepo  resources.Repository
	serviceRepo   services.Repository
	mailer        mailer.Mailer
	adminEmail    string
}

func NewUseCases(
	repo Repository,
	tenantService TenantService,
	resourceRepo resources.Repository,
	serviceRepo services.Repository,
	mailer mailer.Mailer,
	adminEmail string,
) *UseCases {
	return &UseCases{
		repo:          repo,
		tenantService: tenantService,
		resourceRepo:  resourceRepo,
		serviceRepo:   serviceRepo,
		mailer:        mailer,
		adminEmail:    adminEmail,
	}
}

// GetProgress returns the onboarding progress for a tenant.
func (uc *UseCases) GetProgress(ctx context.Context, tenantID uuid.UUID) (*Progress, error) {
	return uc.repo.FindByTenant(ctx, tenantID)
}

// InitProgress creates the onboarding progress record for a new tenant.
// It is idempotent: if a record already exists it returns without error.
func (uc *UseCases) InitProgress(ctx context.Context, tenantID uuid.UUID) error {
	_, err := uc.repo.FindByTenant(ctx, tenantID)
	if err == nil {
		return nil
	}
	_, err = uc.repo.Create(ctx, tenantID)
	return err
}

type StepBarberInput struct {
	TenantID    uuid.UUID
	Name        string
	WorkingDays []int
	StartTime   string
	EndTime     string
}

// CompleteStepBarber creates the barber resource and advances the onboarding step.
func (uc *UseCases) CompleteStepBarber(ctx context.Context, input StepBarberInput) error {
	progress, err := uc.repo.FindByTenant(ctx, input.TenantID)
	if err != nil {
		return err
	}

	if !progress.CanAccessStep(StepBarber) {
		return ErrStepNotAvailable
	}

	res := &resources.Resource{
		ID:        uuid.New(),
		TenantID:  input.TenantID,
		Name:      input.Name,
		Type:      resources.ResourceTypeBarber,
		IsActive:  true,
		SortOrder: 1,
	}

	if err := uc.resourceRepo.Create(ctx, res); err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	for _, day := range input.WorkingDays {
		wh := resources.WorkingHours{
			ID:         uuid.New(),
			ResourceID: res.ID,
			DayOfWeek:  day,
			StartTime:  input.StartTime,
			EndTime:    input.EndTime,
			IsActive:   true,
		}
		if err := uc.resourceRepo.UpsertWorkingHours(ctx, wh); err != nil {
			return fmt.Errorf("create working hours day %d: %w", day, err)
		}
	}

	return uc.repo.AdvanceStep(ctx, input.TenantID)
}

type StepServiceItem struct {
	Name            string
	DurationMinutes int
	BufferMinutes   int
	Price           float64
}

type StepServicesInput struct {
	TenantID uuid.UUID
	Services []StepServiceItem
}

// CompleteStepServices creates the services and assigns them to the first barber resource.
func (uc *UseCases) CompleteStepServices(ctx context.Context, input StepServicesInput) error {
	progress, err := uc.repo.FindByTenant(ctx, input.TenantID)
	if err != nil {
		return err
	}

	if !progress.CanAccessStep(StepServices) {
		return ErrStepNotAvailable
	}

	if len(input.Services) == 0 {
		return ErrServicesRequired
	}

	existingResources, err := uc.resourceRepo.FindByTenant(ctx, input.TenantID)
	if err != nil {
		return fmt.Errorf("find resources: %w", err)
	}

	if len(existingResources) == 0 {
		return ErrBarberRequired
	}

	firstResource := existingResources[0]
	serviceIDs := make([]uuid.UUID, 0, len(input.Services))

	for i, item := range input.Services {
		svc := &services.Service{
			ID:              uuid.New(),
			TenantID:        input.TenantID,
			Name:            item.Name,
			DurationMinutes: item.DurationMinutes,
			BufferMinutes:   item.BufferMinutes,
			Price:           item.Price,
			IsActive:        true,
			SortOrder:       i + 1,
		}

		if err := svc.Validate(); err != nil {
			return fmt.Errorf("invalid service %q: %w", item.Name, err)
		}

		if err := uc.serviceRepo.Create(ctx, svc); err != nil {
			return fmt.Errorf("create service %q: %w", item.Name, err)
		}

		serviceIDs = append(serviceIDs, svc.ID)
	}

	if err := uc.resourceRepo.AssignServices(ctx, firstResource.ID, serviceIDs); err != nil {
		return fmt.Errorf("assign services: %w", err)
	}

	return uc.repo.AdvanceStep(ctx, input.TenantID)
}

type StepWhatsAppInput struct {
	TenantID     uuid.UUID
	ContactEmail string
	Notes        string
}

// CompleteStepWhatsApp submits the WhatsApp activation request and completes onboarding.
func (uc *UseCases) CompleteStepWhatsApp(ctx context.Context, input StepWhatsAppInput) error {
	progress, err := uc.repo.FindByTenant(ctx, input.TenantID)
	if err != nil {
		return err
	}

	if !progress.CanAccessStep(StepWhatsApp) {
		return ErrStepNotAvailable
	}

	tenant, err := uc.tenantService.FindByID(ctx, input.TenantID)
	if err != nil {
		return fmt.Errorf("find tenant: %w", err)
	}

	if err := uc.tenantService.CreateWhatsappConfigPending(ctx, tenants.CreateWhatsappConfigPendingInput{
		TenantID:     input.TenantID,
		ContactEmail: input.ContactEmail,
		Notes:        input.Notes,
	}); err != nil {
		return fmt.Errorf("create whatsapp config: %w", err)
	}

	if err := uc.repo.Complete(ctx, input.TenantID); err != nil {
		return fmt.Errorf("complete onboarding: %w", err)
	}

	go uc.mailer.Send(ctx, mailer.Email{
		To:      input.ContactEmail,
		Subject: "✂️ Estamos configurando tu WhatsApp",
		Body:    buildOwnerRequestEmail(tenant.Name),
	})

	go uc.mailer.Send(ctx, mailer.Email{
		To:      uc.adminEmail,
		Subject: fmt.Sprintf("🔔 Nueva activación pendiente: %s", tenant.Name),
		Body:    buildAdminNotificationEmail(tenant.Name, input.ContactEmail, input.Notes),
	})

	return nil
}

// GetTemplates returns the available service templates for onboarding.
func (uc *UseCases) GetTemplates() map[string][]ServiceTemplate {
	return Templates
}

var (
	ErrStepNotAvailable = onboardingError("step not available yet")
	ErrServicesRequired = onboardingError("at least one service is required")
	ErrBarberRequired   = onboardingError("complete the barber step first")
)

type onboardingError string

func (e onboardingError) Error() string { return string(e) }

func buildOwnerRequestEmail(tenantName string) string {
	return fmt.Sprintf(`
		<h2>¡Hola!</h2>
		<p>Recibimos tu solicitud para activar el WhatsApp de <strong>%s</strong>.</p>
		<p>Nuestro equipo está trabajando en ello.</p>
		<p><strong>Tiempo estimado: 2 horas hábiles.</strong></p>
		<p>Mientras esperas puedes personalizar tu panel.</p>
	`, tenantName)
}

func buildAdminNotificationEmail(tenantName, contactEmail, notes string) string {
	return fmt.Sprintf(`
		<h2>Nueva activación pendiente</h2>
		<p><strong>Barbería:</strong> %s</p>
		<p><strong>Correo:</strong> %s</p>
		<p><strong>Notas:</strong> %s</p>
	`, tenantName, contactEmail, notes)
}
