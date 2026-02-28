package services

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"appointments/internal/models"

	"gorm.io/gorm"
)

// ConversationStep is the FSM state for a conversation.
type ConversationStep string

const (
	StepNew                 ConversationStep = "NEW"
	StepChoosingOption      ConversationStep = "CHOOSING_OPTION"
	StepWaitingName         ConversationStep = "WAITING_NAME"
	StepWaitingDate         ConversationStep = "WAITING_DATE"
	StepManagingAppointment ConversationStep = "MANAGING_APPOINTMENT"
)

// Button IDs for the appointment management interactive message.
const (
	btnConfirmAppointment = "btn_confirm"
	btnCancelAppointment  = "btn_cancel"
)

// TempDataStruct holds transient data between conversation steps.
type TempDataStruct struct {
	DraftName     string `json:"draft_name"`
	AppointmentID uint   `json:"appointment_id,omitempty"`
}

// ---- State machine infrastructure ----

// StepContext carries all dependencies and helpers available to a step handler.
type StepContext struct {
	DB   *gorm.DB
	Conv *models.Conversation
	Msg  models.WhatsAppMessage
}

// Reply sends a WhatsApp message to the conversation's phone number.
func (c *StepContext) Reply(body string) {
	reply(c.Msg.From, body)
}

// Transition persists a new FSM step and optional temp data.
func (c *StepContext) Transition(step ConversationStep, data interface{}) error {
	return updateState(c.DB, c.Conv, step, data)
}

// LoadTempData unmarshals the conversation's stored temp data into dst.
func (c *StepContext) LoadTempData(dst interface{}) error {
	if len(c.Conv.TempData) == 0 {
		return nil
	}
	return json.Unmarshal(c.Conv.TempData, dst)
}

// ReplyButtons sends an interactive button message to the conversation's phone number.
func (c *StepContext) ReplyButtons(body string, buttons []ReplyButton) {
	if err := SendWhatsAppButtons(c.Msg.From, body, buttons); err != nil {
		slog.Error("failed to send whatsapp buttons", "to", c.Msg.From, "error", err)
	}
}

// StepHandlerFunc is the signature every step handler must implement.
// Return a non-nil error only for unexpected system failures; user-input
// errors should reply directly and return nil.
type StepHandlerFunc func(ctx *StepContext) error

// StateMachine maps ConversationSteps to their handlers.
type StateMachine struct {
	handlers map[ConversationStep]StepHandlerFunc
}

// NewStateMachine builds and returns the default appointment booking machine.
func NewStateMachine() *StateMachine {
	sm := &StateMachine{handlers: make(map[ConversationStep]StepHandlerFunc)}
	sm.Register(StepNew, handleNew)
	sm.Register(StepChoosingOption, handleChoosingOption)
	sm.Register(StepWaitingName, handleWaitingName)
	sm.Register(StepWaitingDate, handleWaitingDate)
	sm.Register(StepManagingAppointment, handleManagingAppointment)
	return sm
}

// Register adds or replaces the handler for a given step.
// Returns the machine itself to allow chaining.
func (sm *StateMachine) Register(step ConversationStep, h StepHandlerFunc) *StateMachine {
	sm.handlers[step] = h
	return sm
}

// Process resolves the conversation state and dispatches to the correct handler.
func (sm *StateMachine) Process(db *gorm.DB, msg models.WhatsAppMessage) {
	var conv models.Conversation
	if err := db.FirstOrCreate(&conv, models.Conversation{Phone: msg.From}).Error; err != nil {
		slog.Error("failed to find or create conversation", "phone", msg.From, "error", err)
		return
	}

	slog.Info("processing message", "phone", msg.From, "step", conv.CurrentStep, "input", msg.Text.Body)

	h, ok := sm.handlers[ConversationStep(conv.CurrentStep)]
	if !ok {
		slog.Warn("unknown conversation step, resetting", "phone", msg.From, "step", conv.CurrentStep)
		if err := updateState(db, &conv, StepNew, nil); err != nil {
			slog.Error("failed to reset conversation state", "phone", msg.From, "error", err)
		}
		return
	}

	if err := h(&StepContext{DB: db, Conv: &conv, Msg: msg}); err != nil {
		slog.Error("step handler error", "phone", msg.From, "step", conv.CurrentStep, "error", err)
	}
}

// defaultMachine is the singleton used by ProcessConversation.
var defaultMachine = NewStateMachine()

// ProcessConversation is the public entry point. It delegates to the default state machine.
func ProcessConversation(db *gorm.DB, msg models.WhatsAppMessage) {
	defaultMachine.Process(db, msg)
}

// ---- Step handlers ----

func handleNew(ctx *StepContext) error {
	ctx.Reply("¡Hola! 👋 Bienvenido al sistema de agendamiento.\n\n¿Qué deseas hacer?\n\n*1.* 📅 Agendar una cita\n*2.* 🔍 Consultar mi cita")
	return ctx.Transition(StepChoosingOption, nil)
}

func handleChoosingOption(ctx *StepContext) error {
	switch strings.TrimSpace(ctx.Msg.Text.Body) {
	case "1":
		ctx.Reply("Por favor, escribe tu *Nombre Completo* para continuar:")
		return ctx.Transition(StepWaitingName, nil)
	case "2":
		appt, err := GetLatestAppointment(ctx.DB, ctx.Msg.From)
		if err != nil {
			slog.Error("failed to get latest appointment", "phone", ctx.Msg.From, "error", err)
			ctx.Reply("Error interno consultando tu cita. Intenta más tarde.")
			return nil
		}
		if appt == nil {
			ctx.Reply("No encontramos ninguna cita registrada para tu número.")
			return ctx.Transition(StepNew, nil)
		}
		ctx.ReplyButtons(
			fmt.Sprintf("📋 *Tu cita más reciente:*\n\nCliente: %s\nFecha: %s\nEstado: %s",
				appt.ClientName,
				appt.StartTime.In(bogotaLoc).Format("02/01/2006 03:04 PM"),
				appt.Status,
			),
			[]ReplyButton{
				{ID: btnConfirmAppointment, Title: "✅ Confirmar"},
				{ID: btnCancelAppointment, Title: "❌ Cancelar"},
			},
		)
		return ctx.Transition(StepManagingAppointment, TempDataStruct{AppointmentID: appt.ID})
	default:
		ctx.Reply("Por favor responde *1* para agendar o *2* para consultar tu cita:")
		return nil
	}
}

func handleManagingAppointment(ctx *StepContext) error {
	if ctx.Msg.Type != "interactive" {
		ctx.Reply("Por favor usa los botones para confirmar o cancelar tu cita.")
		return nil
	}

	var data TempDataStruct
	if err := ctx.LoadTempData(&data); err != nil || data.AppointmentID == 0 {
		slog.Error("failed to load appointment ID from temp data", "phone", ctx.Msg.From, "error", err)
		ctx.Reply("Error interno. Por favor comienza de nuevo.")
		return ctx.Transition(StepNew, nil)
	}

	switch ctx.Msg.Interactive.ButtonReply.ID {
	case btnConfirmAppointment:
		if err := UpdateAppointmentStatus(ctx.DB, data.AppointmentID, StatusConfirmed); err != nil {
			slog.Error("failed to confirm appointment", "phone", ctx.Msg.From, "appointment_id", data.AppointmentID, "error", err)
			ctx.Reply("No pudimos confirmar tu cita. Intenta más tarde.")
			return nil
		}
		slog.Info("appointment confirmed", "phone", ctx.Msg.From, "appointment_id", data.AppointmentID)
		ctx.Reply("✅ ¡Tu cita ha sido *confirmada*! Te esperamos.")

	case btnCancelAppointment:
		if err := UpdateAppointmentStatus(ctx.DB, data.AppointmentID, StatusCancelled); err != nil {
			slog.Error("failed to cancel appointment", "phone", ctx.Msg.From, "appointment_id", data.AppointmentID, "error", err)
			ctx.Reply("No pudimos cancelar tu cita. Intenta más tarde.")
			return nil
		}
		slog.Info("appointment cancelled", "phone", ctx.Msg.From, "appointment_id", data.AppointmentID)
		ctx.Reply("❌ Tu cita ha sido *cancelada*.")

	default:
		slog.Warn("unknown button reply", "phone", ctx.Msg.From, "button_id", ctx.Msg.Interactive.ButtonReply.ID)
		ctx.Reply("Opción no reconocida. Por favor usa los botones.")
		return nil
	}

	return ctx.Transition(StepNew, nil)
}

func handleWaitingName(ctx *StepContext) error {
	name := ctx.Msg.Text.Body
	if len(name) < 3 {
		ctx.Reply("El nombre es muy corto. Por favor intenta de nuevo:")
		return nil
	}

	ctx.Reply(fmt.Sprintf(
		"Gracias %s. 🗓️\n\nPor favor escribe la fecha y hora deseada en formato *DD/MM HH:MM*.\nEjemplo: *28/02 15:00* (para el 28 de feb a las 3 PM).",
		name,
	))
	return ctx.Transition(StepWaitingDate, TempDataStruct{DraftName: name})
}

func handleWaitingDate(ctx *StepContext) error {
	bookedTime, err := ParseAppointmentTime(ctx.Msg.Text.Body)
	if err != nil {
		ctx.Reply("⚠️ Formato inválido o fecha pasada.\nUsa el formato: *DD/MM HH:MM* (Ej: 28/02 15:00)")
		return nil
	}

	isFree, err := CheckAvailability(ctx.DB, bookedTime)
	if err != nil {
		slog.Error("failed to check availability", "phone", ctx.Msg.From, "error", err)
		ctx.Reply("Error interno verificando agenda. Intenta más tarde.")
		return nil
	}
	if !isFree {
		ctx.Reply("❌ Ese horario ya está ocupado.\nPor favor escribe otra hora (Ej: 28/02 16:00):")
		return nil
	}

	var data TempDataStruct
	if err := ctx.LoadTempData(&data); err != nil {
		slog.Error("failed to unmarshal temp data, resetting conversation", "phone", ctx.Msg.From, "error", err)
		ctx.Reply("Error interno recuperando datos. Por favor comienza de nuevo escribiendo tu nombre:")
		return ctx.Transition(StepNew, nil)
	}

	if data.DraftName == "" {
		slog.Warn("draft name missing, resetting conversation", "phone", ctx.Msg.From)
		ctx.Reply("No encontramos tu nombre guardado. Por favor comienza de nuevo:")
		return ctx.Transition(StepNew, nil)
	}

	if err := CreateAppointment(ctx.DB, ctx.Msg.From, data.DraftName, bookedTime); err != nil {
		slog.Error("failed to create appointment", "phone", ctx.Msg.From, "name", data.DraftName, "booked_time", bookedTime, "error", err)
		ctx.Reply("No pudimos guardar tu cita. Intenta de nuevo.")
		return nil
	}

	slog.Info("appointment created", "phone", ctx.Msg.From, "name", data.DraftName, "booked_time", bookedTime)
	ctx.Reply(fmt.Sprintf(
		"✅ *¡Cita Confirmada!*\n\nCliente: %s\nFecha: %s\n\nTe esperamos.",
		data.DraftName,
		bookedTime.Format("02/01/2006 03:04 PM"),
	))
	return ctx.Transition(StepNew, TempDataStruct{})
}

// ---- Unexported helpers ----

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
