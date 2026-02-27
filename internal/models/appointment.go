package models

import (
	"time"

	"gorm.io/gorm"
)

type Appointment struct {
	gorm.Model
	ClientPhone string    `json:"client_phone" gorm:"index"`
	ClientName  string    `json:"client_name"`
	StartTime   time.Time `json:"start_time" gorm:"index"`
	EndTime     time.Time `json:"end_time"`
	Status      string    `json:"status" gorm:"default:'CONFIRMED'"`
}
