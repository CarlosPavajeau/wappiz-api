package onboarding

import (
	"time"

	"github.com/google/uuid"
)


type Step int

const (
	StepAccount  Step = 1
	StepBarber   Step = 2
	StepServices Step = 3
	StepWhatsApp Step = 4
)

type Progress struct {
	ID          uuid.UUID
	TenantID    uuid.UUID
	CurrentStep Step
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (p *Progress) IsCompleted() bool {
	return p.CompletedAt != nil
}

func (p *Progress) CanAccessStep(step Step) bool {
	return step <= p.CurrentStep
}

func (p *Progress) Advance() {
	if p.CurrentStep < StepWhatsApp {
		p.CurrentStep++
	}
}

type ServiceTemplate struct {
	Name            string  `json:"name"`
	DurationMinutes int     `json:"duration_minutes"`
	BufferMinutes   int     `json:"buffer_minutes"`
	Price           float64 `json:"price"`
}

var Templates = map[string][]ServiceTemplate{
	"basic": {
		{Name: "Corte normal", DurationMinutes: 30, BufferMinutes: 5, Price: 15000},
		{Name: "Corte + barba", DurationMinutes: 45, BufferMinutes: 5, Price: 25000},
	},
	"complete": {
		{Name: "Corte normal", DurationMinutes: 30, BufferMinutes: 5, Price: 15000},
		{Name: "Corte + barba", DurationMinutes: 45, BufferMinutes: 5, Price: 25000},
		{Name: "Lavado", DurationMinutes: 20, BufferMinutes: 5, Price: 10000},
		{Name: "Afeitado", DurationMinutes: 30, BufferMinutes: 5, Price: 20000},
	},
	"manual": {},
}
