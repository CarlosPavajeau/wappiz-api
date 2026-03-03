package onboarding

import (
	"context"
	"fmt"
	"log"

	"appointments/internal/features/resources"
	"appointments/internal/features/services"
	"appointments/internal/features/tenants"
	"appointments/internal/platform/mailer"

	"github.com/google/uuid"
)

type UseCases struct {
	repo         Repository
	tenantRepo   tenants.Repository
	resourceRepo resources.Repository
	serviceRepo  services.Repository
	mailer       mailer.Mailer
	adminEmail   string
}

func NewUseCases(
	repo Repository,
	tenantRepo tenants.Repository,
	resourceRepo resources.Repository,
	serviceRepo services.Repository,
	mailer mailer.Mailer,
	adminEmail string,
) *UseCases {
	return &UseCases{
		repo:         repo,
		tenantRepo:   tenantRepo,
		resourceRepo: resourceRepo,
		serviceRepo:  serviceRepo,
		mailer:       mailer,
		adminEmail:   adminEmail,
	}
}

func (uc *UseCases) GetProgress(ctx context.Context, tenantID uuid.UUID) (*Progress, error) {
	return uc.repo.FindByTenant(ctx, tenantID)
}

func (uc *UseCases) InitProgress(ctx context.Context, tenantID uuid.UUID) (*Progress, error) {
	return uc.repo.Create(ctx, tenantID)
}

type StepBarberInput struct {
	TenantID    uuid.UUID
	Name        string
	WorkingDays []int
	StartTime   string
	EndTime     string
}

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
	var serviceIDs []uuid.UUID

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

func (uc *UseCases) CompleteStepWhatsApp(ctx context.Context, input StepWhatsAppInput) error {
	log.Printf("onboarding: step4 start tenant_id=%s", input.TenantID)

	progress, err := uc.repo.FindByTenant(ctx, input.TenantID)
	if err != nil {
		log.Printf("onboarding: step4 find_progress error tenant_id=%s err=%v", input.TenantID, err)
		return err
	}
	log.Printf("onboarding: step4 progress current_step=%d tenant_id=%s", progress.CurrentStep, input.TenantID)

	if !progress.CanAccessStep(StepWhatsApp) {
		log.Printf("onboarding: step4 not_available current_step=%d tenant_id=%s", progress.CurrentStep, input.TenantID)
		return ErrStepNotAvailable
	}

	tenant, err := uc.tenantRepo.FindByID(ctx, input.TenantID)
	if err != nil {
		log.Printf("onboarding: step4 find_tenant error tenant_id=%s err=%v", input.TenantID, err)
		return fmt.Errorf("find tenant: %w", err)
	}
	log.Printf("onboarding: step4 tenant_found name=%q tenant_id=%s", tenant.Name, input.TenantID)

	if err := uc.tenantRepo.CreateWhatsappConfigPending(ctx, tenants.CreateWhatsappConfigPendingInput{
		TenantID:     input.TenantID,
		ContactEmail: input.ContactEmail,
		Notes:        input.Notes,
	}); err != nil {
		log.Printf("onboarding: step4 create_whatsapp_config_pending error tenant_id=%s err=%v", input.TenantID, err)
		return fmt.Errorf("create whatsapp config: %w", err)
	}
	log.Printf("onboarding: step4 whatsapp_config_pending created tenant_id=%s", input.TenantID)

	if err := uc.repo.Complete(ctx, input.TenantID); err != nil {
		log.Printf("onboarding: step4 complete_onboarding error tenant_id=%s err=%v", input.TenantID, err)
		return fmt.Errorf("complete onboarding: %w", err)
	}
	log.Printf("onboarding: step4 onboarding completed tenant_id=%s", input.TenantID)

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

type ActivateTenantInput struct {
	TenantID           uuid.UUID
	PhoneNumberID      string
	DisplayPhoneNumber string
	WABAID             string
	AccessToken        string
}

func (uc *UseCases) ActivateTenant(ctx context.Context, input ActivateTenantInput) error {
	tenant, err := uc.tenantRepo.FindByID(ctx, input.TenantID)
	if err != nil {
		return fmt.Errorf("find tenant: %w", err)
	}

	if err := uc.tenantRepo.ActivateWhatsappConfig(ctx, tenants.ActivateWhatsappConfigInput{
		TenantID:           input.TenantID,
		PhoneNumberID:      input.PhoneNumberID,
		DisplayPhoneNumber: input.DisplayPhoneNumber,
		WABAID:             input.WABAID,
		AccessToken:        input.AccessToken,
	}); err != nil {
		return fmt.Errorf("activate whatsapp config: %w", err)
	}

	go uc.mailer.Send(ctx, mailer.Email{
		To:      tenant.Settings.ContactEmail,
		Subject: "🎉 Tu WhatsApp ya está activo — Turnio",
		Body:    buildActivationEmail(tenant.Name, input.DisplayPhoneNumber),
	})

	return nil
}

func buildActivationEmail(tenantName, phoneNumber string) string {
	waLink := "https://wa.me/" + sanitizePhone(phoneNumber)
	return fmt.Sprintf(`
		<h2>🎉 ¡Tu barbería ya puede recibir citas!</h2>
		<p>Hola <strong>%s</strong>,</p>
		<p>Tu número de WhatsApp ya está listo.</p>
		<h3>📱 Número de tu barbería:<br>%s</h3>
		<p>Dale este número a tus clientes o comparte el enlace directo:</p>
		<p><a href="%s">%s</a></p>
	`, tenantName, phoneNumber, waLink, waLink)
}

func sanitizePhone(phone string) string {
	// Remover +, espacios y guiones para el enlace wa.me
	result := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			result += string(ch)
		}
	}
	return result
}

func (uc *UseCases) GetTemplates() map[string][]ServiceTemplate {
	return Templates
}

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

var (
	ErrStepNotAvailable = onboardingError("step not available yet")
	ErrServicesRequired = onboardingError("at least one service is required")
	ErrBarberRequired   = onboardingError("complete the barber step first")
)

type onboardingError string

func (e onboardingError) Error() string { return string(e) }
