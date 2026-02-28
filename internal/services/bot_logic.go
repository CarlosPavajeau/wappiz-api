package services

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"appointments/internal/models"

	"gorm.io/gorm"
)

// ConversationStep is the FSM state for a conversation.
type ConversationStep string

const (
	StepNew         ConversationStep = "NEW"
	StepWaitingName ConversationStep = "WAITING_NAME"
	StepWaitingDate ConversationStep = "WAITING_DATE"
)

// TempDataStruct holds transient data between conversation steps.
type TempDataStruct struct {
	DraftName string `json:"draft_name"`
}

func ProcessConversation(db *gorm.DB, msg models.WhatsAppMessage) {
	var conv models.Conversation

	if err := db.FirstOrCreate(&conv, models.Conversation{Phone: msg.From}).Error; err != nil {
		slog.Error("failed to find or create conversation", "phone", msg.From, "error", err)
		return
	}

	slog.Info("processing message", "phone", msg.From, "step", conv.CurrentStep, "input", msg.Text.Body)

	switch ConversationStep(conv.CurrentStep) {
	case StepNew:
		reply(msg.From, "¡Hola! 👋 Bienvenido al sistema de agendamiento.\n\nPor favor, escribe tu *Nombre Completo* para comenzar:")
		if err := updateState(db, &conv, StepWaitingName, nil); err != nil {
			slog.Error("failed to update state", "phone", msg.From, "target_step", StepWaitingName, "error", err)
		}

	case StepWaitingName:
		name := msg.Text.Body
		if len(name) < 3 {
			reply(msg.From, "El nombre es muy corto. Por favor intenta de nuevo:")
			return
		}

		reply(msg.From, fmt.Sprintf(
			"Gracias %s. 🗓️\n\nPor favor escribe la fecha y hora deseada en formato *DD/MM HH:MM*.\nEjemplo: *28/02 15:00* (para el 28 de feb a las 3 PM).",
			name,
		))

		if err := updateState(db, &conv, StepWaitingDate, TempDataStruct{DraftName: name}); err != nil {
			slog.Error("failed to update state", "phone", msg.From, "target_step", StepWaitingDate, "error", err)
		}

	case StepWaitingDate:
		bookedTime, err := ParseAppointmentTime(msg.Text.Body)
		if err != nil {
			reply(msg.From, "⚠️ Formato inválido o fecha pasada.\nUsa el formato: *DD/MM HH:MM* (Ej: 28/02 15:00)")
			return
		}

		isFree, err := CheckAvailability(db, bookedTime)
		if err != nil {
			slog.Error("failed to check availability", "phone", msg.From, "error", err)
			reply(msg.From, "Error interno verificando agenda. Intenta más tarde.")
			return
		}
		if !isFree {
			reply(msg.From, "❌ Ese horario ya está ocupado.\nPor favor escribe otra hora (Ej: 28/02 16:00):")
			return
		}

		var data TempDataStruct
		if len(conv.TempData) > 0 {
			if err := json.Unmarshal(conv.TempData, &data); err != nil {
				slog.Error("failed to unmarshal temp data, resetting conversation", "phone", msg.From, "error", err)
				reply(msg.From, "Error interno recuperando datos. Por favor comienza de nuevo escribiendo tu nombre:")
				_ = updateState(db, &conv, StepNew, nil)
				return
			}
		}

		if data.DraftName == "" {
			slog.Warn("draft name missing, resetting conversation", "phone", msg.From)
			reply(msg.From, "No encontramos tu nombre guardado. Por favor comienza de nuevo:")
			_ = updateState(db, &conv, StepNew, nil)
			return
		}

		if err := CreateAppointment(db, msg.From, data.DraftName, bookedTime); err != nil {
			slog.Error("failed to create appointment", "phone", msg.From, "name", data.DraftName, "booked_time", bookedTime, "error", err)
			reply(msg.From, "No pudimos guardar tu cita. Intenta de nuevo.")
			return
		}

		slog.Info("appointment created", "phone", msg.From, "name", data.DraftName, "booked_time", bookedTime)

		reply(msg.From, fmt.Sprintf(
			"✅ *¡Cita Confirmada!*\n\nCliente: %s\nFecha: %s\n\nTe esperamos.",
			data.DraftName,
			bookedTime.Format("02/01/2006 03:04 PM"),
		))

		if err := updateState(db, &conv, StepNew, TempDataStruct{}); err != nil {
			slog.Error("failed to reset conversation state", "phone", msg.From, "error", err)
		}

	default:
		slog.Warn("unknown conversation step, resetting", "phone", msg.From, "step", conv.CurrentStep)
		reply(msg.From, "Sesión reiniciada. Escribe tu nombre para agendar:")
		if err := updateState(db, &conv, StepWaitingName, nil); err != nil {
			slog.Error("failed to reset conversation state", "phone", msg.From, "error", err)
		}
	}
}

// reply sends a WhatsApp message and logs any delivery failure.
func reply(to, body string) {
	if err := SendWhatsAppMessage(to, body); err != nil {
		slog.Error("failed to send whatsapp message", "to", to, "error", err)
	}
}

// updateState persists the new FSM step and optional temp data.
func updateState(db *gorm.DB, conv *models.Conversation, step ConversationStep, data interface{}) error {
	conv.CurrentStep = string(step)

	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("marshal temp data: %w", err)
		}
		conv.TempData = jsonData
	} else {
		conv.TempData = []byte("{}")
	}

	if err := db.Save(conv).Error; err != nil {
		return fmt.Errorf("save conversation: %w", err)
	}
	return nil
}
