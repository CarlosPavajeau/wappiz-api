package scheduling

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"wappiz/internal/features/appointments"
	"wappiz/internal/features/customers"
	"wappiz/internal/features/tenants"
	"wappiz/internal/platform/mailer"
	"wappiz/internal/platform/whatsapp"
)

// AppointmentRepository defines the appointment operations needed by NoShowTracker.
type AppointmentRepository interface {
	FindUnattended(ctx context.Context) ([]appointments.Appointment, error)
	FindRecentlyCancelled(ctx context.Context) ([]appointments.Appointment, error)
	UpdateStatusWithHistory(ctx context.Context, id uuid.UUID, status string, changedBy *string, reason string, h *appointments.AppointmentStatusHistory) error
}

type NoShowTracker struct {
	appointments AppointmentRepository
	customers    customers.Repository
	tenants      tenants.Repository
	wa           whatsapp.Client
	mailer       mailer.Mailer
}

func NewNoShowTracker(
	appointments AppointmentRepository,
	customers customers.Repository,
	tenants tenants.Repository,
	wa whatsapp.Client,
	mailer mailer.Mailer,
) *NoShowTracker {
	return &NoShowTracker{
		appointments: appointments,
		customers:    customers,
		tenants:      tenants,
		wa:           wa,
		mailer:       mailer,
	}
}

func (t *NoShowTracker) Run(ctx context.Context) {
	// Step 1: detect unattended appointments and mark as no_show
	unattended, err := t.appointments.FindUnattended(ctx)
	if err != nil {
		log.Printf("[no_show_tracker] find unattended error: %v", err)
		return
	}

	// customerID -> tenantID for Step 2 evaluation
	affected := make(map[uuid.UUID]uuid.UUID)
	for _, a := range unattended {
		h := &appointments.AppointmentStatusHistory{
			ID:            uuid.New(),
			AppointmentID: a.ID,
			FromStatus:    "confirmed",
			ToStatus:      "no_show",
			ChangedByRole: "system",
			Reason:        "Auto-detected: customer did not check in",
		}
		if err := t.appointments.UpdateStatusWithHistory(ctx, a.ID, "no_show", nil, h.Reason, h); err != nil {
			log.Printf("[no_show_tracker] update status error | appointmentID=%s tenantID=%s err=%v", a.ID, a.TenantID, err)
			continue
		}
		log.Printf("[no_show_tracker] auto no-show detected | appointmentID=%s tenantID=%s", a.ID, a.TenantID)
		affected[a.CustomerID] = a.TenantID
	}

	// Step 2: evaluate each affected customer
	processed := make(map[uuid.UUID]bool)
	for customerID, tenantID := range affected {
		t.evaluateCustomer(ctx, tenantID, customerID, "no_show")
		processed[customerID] = true
	}

	// Step 3: detect late cancellations from last 10 minutes
	recentlyCancelled, err := t.appointments.FindRecentlyCancelled(ctx)
	if err != nil {
		log.Printf("[no_show_tracker] find recently cancelled error: %v", err)
		return
	}

	for _, a := range recentlyCancelled {
		if processed[a.CustomerID] {
			continue
		}

		tenant, err := t.tenants.FindByID(ctx, a.TenantID)
		if err != nil {
			log.Printf("[no_show_tracker] find tenant error | appointmentID=%s err=%v", a.ID, err)
			continue
		}

		lateHours := tenant.Settings.LateCancelHours
		if lateHours == 0 {
			lateHours = 2
		}

		if a.CancelledAt == nil {
			continue
		}
		if a.StartsAt.Sub(*a.CancelledAt).Hours() >= float64(lateHours) {
			continue // not a late cancellation
		}

		log.Printf("[no_show_tracker] late cancel detected | appointmentID=%s customerID=%s", a.ID, a.CustomerID)
		t.evaluateCustomer(ctx, a.TenantID, a.CustomerID, "late_cancel")
		processed[a.CustomerID] = true
	}
}

func (t *NoShowTracker) evaluateCustomer(ctx context.Context, tenantID, customerID uuid.UUID, reason string) {
	customer, err := t.customers.FindByID(ctx, customerID)
	if err != nil {
		log.Printf("[no_show_tracker] find customer error | customerID=%s err=%v", customerID, err)
		return
	}

	if customer.IsBlocked {
		log.Printf("[no_show_tracker] customer already blocked, skipping | customerID=%s", customerID)
		return
	}

	tenant, err := t.tenants.FindByID(ctx, tenantID)
	if err != nil {
		log.Printf("[no_show_tracker] find tenant error | customerID=%s err=%v", customerID, err)
		return
	}

	lateHours := tenant.Settings.LateCancelHours
	if lateHours == 0 {
		lateHours = 2
	}

	noShows, lateCancels, err := t.customers.GetNoShowSummary(ctx, tenantID, customerID, lateHours)
	if err != nil {
		log.Printf("[no_show_tracker] get no-show summary error | customerID=%s err=%v", customerID, err)
		return
	}

	autoBlockNoShows := tenant.Settings.AutoBlockAfterNoShows
	if autoBlockNoShows == 0 {
		autoBlockNoShows = 3
	}
	autoBlockLateCancel := tenant.Settings.AutoBlockAfterLateCancel
	if autoBlockLateCancel == 0 {
		autoBlockLateCancel = 3
	}

	autoBlockThreshold := autoBlockNoShows
	if autoBlockLateCancel < autoBlockThreshold {
		autoBlockThreshold = autoBlockLateCancel
	}
	warningThreshold := autoBlockThreshold - 1
	totalEvents := noShows + lateCancels

	waConfig, err := t.tenants.FindWhatsappConfig(ctx, tenantID)
	if err != nil {
		log.Printf("[no_show_tracker] find whatsapp config error | tenantID=%s err=%v", tenantID, err)
		return
	}

	customerName := customer.DisplayName()

	if totalEvents >= autoBlockThreshold {
		if err := t.customers.Block(ctx, customerID, tenantID); err != nil {
			log.Printf("[no_show_tracker] block customer error | customerID=%s err=%v", customerID, err)
			return
		}
		log.Printf("[no_show_tracker] customer blocked | customerID=%s reason=%s count=%d", customerID, reason, totalEvents)

		ownerMsg := buildOwnerBlockNotification(customerName, reason, totalEvents)
		t.notifyOwner(ctx, tenant, waConfig, ownerMsg, buildOwnerBlockEmailSubject(customerName), ownerMsg)

		if tenant.Settings.SendWarningBeforeBlock {
			customerMsg := buildCustomerBlockMessage(tenant.Name)
			if err := t.wa.SendText(ctx, customer.PhoneNumber, waConfig.PhoneNumberID, waConfig.AccessToken, customerMsg); err != nil {
				log.Printf("[no_show_tracker] customer block notify error | customerID=%s err=%v", customerID, err)
			}
		}

	} else if totalEvents == warningThreshold && tenant.Settings.SendWarningBeforeBlock {
		remaining := autoBlockThreshold - totalEvents
		log.Printf("[no_show_tracker] warning sent | customerID=%s remaining=%d", customerID, remaining)

		customerMsg := buildCustomerWarningMessage(tenant.Name, remaining)
		if err := t.wa.SendText(ctx, customer.PhoneNumber, waConfig.PhoneNumberID, waConfig.AccessToken, customerMsg); err != nil {
			log.Printf("[no_show_tracker] customer warning error | customerID=%s err=%v", customerID, err)
		}

		ownerMsg := buildOwnerWarningNotification(customerName, reason, totalEvents, autoBlockThreshold)
		t.notifyOwner(ctx, tenant, waConfig, ownerMsg, buildOwnerWarningEmailSubject(customerName), ownerMsg)
	}
}

// notifyOwner sends a WhatsApp message when ownerPhone is configured, or falls
// back to an email to contactEmail. Both channels are skipped silently if
// neither is configured.
func (t *NoShowTracker) notifyOwner(
	ctx context.Context,
	tenant *tenants.Tenant,
	waConfig *tenants.WhatsappConfig,
	waBody, emailSubject, emailBody string,
) {
	if tenant.Settings.OwnerPhone != "" {
		if err := t.wa.SendText(ctx, tenant.Settings.OwnerPhone, waConfig.PhoneNumberID, waConfig.AccessToken, waBody); err != nil {
			log.Printf("[no_show_tracker] owner whatsapp notify error | tenantID=%s err=%v", tenant.ID, err)
		}
		return
	}
	if tenant.Settings.ContactEmail != "" {
		email := mailer.Email{
			To:      tenant.Settings.ContactEmail,
			Subject: emailSubject,
			Body:    emailBody,
		}
		if err := t.mailer.Send(ctx, email); err != nil {
			log.Printf("[no_show_tracker] owner email notify error | tenantID=%s err=%v", tenant.ID, err)
		}
	}
}

func buildOwnerBlockNotification(customerName, reason string, count int) string {
	eventLabel := "ausencias"
	if reason == "late_cancel" {
		eventLabel = "cancelaciones tardías"
	}
	return fmt.Sprintf("⚠️ *%s* ha sido bloqueado automáticamente después de %d %s.", customerName, count, eventLabel)
}

func buildCustomerBlockMessage(tenantName string) string {
	return fmt.Sprintf(
		"Hola, lamentablemente hemos tenido que suspender tu acceso para agendar citas en *%s* debido a ausencias repetidas. Comunícate directamente con el negocio.",
		tenantName,
	)
}

func buildCustomerWarningMessage(tenantName string, remaining int) string {
	return fmt.Sprintf(
		"Hola, hemos registrado ausencias en tus citas con *%s*. Por favor recuerda cancelar con anticipación. %d ausencia(s) más podría suspender tu acceso.",
		tenantName, remaining,
	)
}

func buildOwnerWarningNotification(customerName, reason string, count, threshold int) string {
	eventLabel := "ausencias"
	if reason == "late_cancel" {
		eventLabel = "cancelaciones tardías"
	}
	return fmt.Sprintf("⚠️ Aviso: *%s* lleva %d %s. Se bloqueará al alcanzar %d.", customerName, count, eventLabel, threshold)
}

func buildOwnerBlockEmailSubject(customerName string) string {
	return fmt.Sprintf("Cliente bloqueado automáticamente: %s", customerName)
}

func buildOwnerWarningEmailSubject(customerName string) string {
	return fmt.Sprintf("Aviso de ausencias: %s", customerName)
}
