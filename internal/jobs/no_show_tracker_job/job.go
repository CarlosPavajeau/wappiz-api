package no_show_tracker_job

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
	"wappiz/pkg/crypto"
	"wappiz/pkg/db"
	"wappiz/pkg/logger"
	"wappiz/pkg/whatsapp"

	"github.com/google/uuid"
)

type job struct {
	db            db.Database
	whatsapp      whatsapp.Client
	encryptionKey []byte
}

func New(cfg Config) *job {
	return &job{
		db:            cfg.DB,
		whatsapp:      cfg.Whatsapp,
		encryptionKey: cfg.EncryptionKey,
	}
}

func (j *job) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	logger.Info("[no_show_tracker] started")

	for {
		select {
		case <-ctx.Done():
			logger.Info("[no_show_tracker] stopped")
			return
		case <-ticker.C:
			if err := j.process(ctx); err != nil {
				logger.Error("[no_show_tracker] failed to process job",
					"err", err)
			}
		}
	}
}

func (j *job) process(ctx context.Context) error {
	unattended, err := db.Query.FindUnattendedAppointments(ctx, j.db.Primary())
	if err != nil {
		logger.Warn("[no_show_tracker] failed to find unattended appointments",
			"err", err)
		return err
	}

	affected := make(map[uuid.UUID]uuid.UUID)
	for _, a := range unattended {
		if err := db.Query.UpdateAppointment(ctx, j.db.Primary(), db.UpdateAppointmentParams{
			Status:       db.AppointmentStatusNoShow,
			CancelledBy:  sql.NullString{},
			CancelReason: sql.NullString{},
			CompletedAt:  sql.NullTime{},
			ID:           a.ID,
		}); err != nil {
			logger.Warn("[no_show_tracker] failed to update appointment",
				"err", err)
			continue
		}

		if err := db.Query.InsertAppointmentStatusHistory(ctx, j.db.Primary(), db.InsertAppointmentStatusHistoryParams{
			ID:            uuid.New(),
			AppointmentID: a.ID,
			FromStatus:    a.Status,
			ToStatus:      db.AppointmentStatusNoShow,
			ChangedBy:     sql.NullString{},
			ChangedByRole: sql.NullString{String: "system", Valid: true},
			Reason:        sql.NullString{String: "Auto-detected: customer did not check in", Valid: true},
		}); err != nil {
			logger.Warn("[no_show_tracker] failed to insert appointment history",
				"err", err)
			continue
		}

		affected[a.CustomerID] = a.TenantID
	}

	processed := make(map[uuid.UUID]bool)
	for customerID, tenantID := range affected {
		j.evaluateCustomer(ctx, tenantID, customerID)
		processed[customerID] = true
	}

	recentlyCancelled, err := db.Query.FindRecentlyCancelledAppointments(ctx, j.db.Primary())
	if err != nil {
		logger.Warn("[no_show_tracker] failed to find recently cancelled appointments",
			"err", err)
		return err
	}

	for _, a := range recentlyCancelled {
		if processed[a.CustomerID] {
			continue
		}

		tenant, err := db.Query.FindTenantByID(ctx, j.db.Primary(), a.TenantID)
		if err != nil {
			logger.Warn("[no_show_tracker] failed to find tenant",
				"err", err)
			continue
		}

		var tenantSettings db.TenantSettings
		if err := json.Unmarshal(tenant.Settings, &tenantSettings); err != nil {
			logger.Warn("[no_show_tracker] failed to unmarshal tenant settings",
				"err", err)
			continue
		}

		lateHours := tenantSettings.LateCancelHours
		if lateHours == 0 {
			lateHours = 2
		}

		if !a.CancelledAt.Valid {
			continue
		}

		if a.StartsAt.Sub(a.CancelledAt.Time).Hours() >= float64(lateHours) {
			continue // not a late cancellation
		}

		j.evaluateCustomer(ctx, a.TenantID, a.CustomerID)
		processed[a.CustomerID] = true
	}

	return nil
}

func (j *job) evaluateCustomer(ctx context.Context, tenantID, customerID uuid.UUID) {
	customer, err := db.Query.FindCustomerByID(ctx, j.db.Primary(), customerID)
	if err != nil {
		logger.Warn("[no_show_tracker] failed to find customer",
			"customer_id", customerID,
			"err", err)
		return
	}

	if customer.IsBlocked { // Don't notify blocked customers
		return
	}

	tenant, err := db.Query.FindTenantByID(ctx, j.db.Primary(), tenantID)
	if err != nil {
		logger.Warn("[no_show_tracker] failed to find tenant",
			"tenant_id", tenantID,
			"err", err)
		return
	}

	var tenantSettings db.TenantSettings
	if err := json.Unmarshal(tenant.Settings, &tenantSettings); err != nil {
		logger.Warn("[no_show_tracker] failed to unmarshal tenant settings",
			"err", err)
		return
	}

	noShows, err := db.Query.CountCustomerNoShows(ctx, j.db.Primary(), db.CountCustomerNoShowsParams{
		TenantID:   tenantID,
		CustomerID: customerID,
	})

	if err != nil {
		logger.Warn("[no_show_tracker] failed to count customer no shows",
			"err", err)
		return
	}

	lateCancels, err := db.Query.CountCustomerLateCancels(ctx, j.db.Primary(), db.CountCustomerLateCancelsParams{
		TenantID:   tenantID,
		CustomerID: customerID,
	})
	if err != nil {
		logger.Warn("[no_show_tracker] failed to count customer late cancels",
			"err", err)
		return
	}

	lateHours := tenantSettings.LateCancelHours
	if lateHours == 0 {
		lateHours = 2
	}

	autoBlockNoShows := tenantSettings.AutoBlockAfterNoShows
	if autoBlockNoShows == 0 {
		autoBlockNoShows = 3
	}
	autoBlockLateCancel := tenantSettings.AutoBlockAfterLateCancel
	if autoBlockLateCancel == 0 {
		autoBlockLateCancel = 3
	}

	autoBlockThreshold := autoBlockNoShows
	if autoBlockLateCancel < autoBlockThreshold {
		autoBlockThreshold = autoBlockLateCancel
	}

	warningThreshold := autoBlockThreshold - 1
	totalEvents := noShows + lateCancels

	waConfig, err := db.Query.FindTenantWhatsappConfig(ctx, j.db.Primary(), tenantID)
	if err != nil {
		logger.Warn("[no_show_tracker] failed to find tenant whatsapp config",
			"tenant_id", tenantID,
			"err", err)
		return
	}

	if !waConfig.PhoneNumberID.Valid || !waConfig.AccessToken.Valid {
		return
	}

	phoneNumberID := waConfig.PhoneNumberID.String
	accessToken, _ := crypto.Decrypt(waConfig.AccessToken.String, j.encryptionKey)

	if totalEvents >= int64(autoBlockThreshold) {
		if err := db.Query.BlockCustomer(ctx, j.db.Primary(), db.BlockCustomerParams{
			ID:       customerID,
			TenantID: tenantID,
		}); err != nil {
			logger.Warn("[no_show_tracker] failed to block customer",
				"err", err)
			return
		}

		// TODO: Notify to tenant owner about customer's block

		if tenantSettings.SendWarningBeforeBlock {
			customerMsg := buildCustomerBlockMessage(tenant.Name)
			if err := j.whatsapp.SendText(ctx, customer.PhoneNumber, phoneNumberID, accessToken, customerMsg); err != nil {
				logger.Warn("[no_show_tracker] failed to send block notification to customer",
					"customer_id", customerID,
					"err", err)
			}
		}
	} else if totalEvents == int64(warningThreshold) && tenantSettings.SendWarningBeforeBlock {
		remaining := int64(autoBlockThreshold) - totalEvents
		customerMsg := buildCustomerWarningMessage(tenant.Name, remaining)

		if err := j.whatsapp.SendText(ctx, customer.PhoneNumber, phoneNumberID, accessToken, customerMsg); err != nil {
			logger.Warn("[no_show_tracker] failed to send block notification to customer",
				"customer_id", customerID,
				"err", err)
		}
	}
}

func buildCustomerBlockMessage(tenantName string) string {
	return fmt.Sprintf(
		"Hola, lamentablemente hemos tenido que suspender tu acceso para agendar citas en *%s* debido a ausencias repetidas. Comunícate directamente con el negocio.",
		tenantName,
	)
}

func buildCustomerWarningMessage(tenantName string, remaining int64) string {
	return fmt.Sprintf(
		"Hola, hemos registrado ausencias en tus citas con *%s*. Por favor recuerda cancelar con anticipación. %d ausencia(s) más podría suspender tu acceso.",
		tenantName, remaining,
	)
}
