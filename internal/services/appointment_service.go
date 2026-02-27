package services

import (
	"errors"
	"fmt"
	"time"

	"appointments/internal/models"
	"gorm.io/gorm"
)

const (
	AppointmentDuration = 1 * time.Hour
	TimeZone            = "America/Bogota"
)

func ParseAppointmentTime(input string) (time.Time, error) {
	loc, err := time.LoadLocation(TimeZone)
	if err != nil {
		return time.Time{}, fmt.Errorf("error cargando zona horaria: %v", err)
	}

	layout := "02/01 15:04"
	parsed, err := time.ParseInLocation(layout, input, loc)
	if err != nil {
		return time.Time{}, errors.New("formato inválido")
	}

	now := time.Now().In(loc)
	finalTime := time.Date(now.Year(), parsed.Month(), parsed.Day(), parsed.Hour(), parsed.Minute(), 0, 0, loc)

	if finalTime.Before(now) {
		return time.Time{}, errors.New("la fecha ya pasó")
	}

	return finalTime, nil
}

func CheckAvailability(db *gorm.DB, startTime time.Time) (bool, error) {
	endTime := startTime.Add(AppointmentDuration)
	var count int64

	err := db.Model(&models.Appointment{}).
		Where("status = ?", "CONFIRMED").
		Where("start_time < ? AND end_time > ?", endTime, startTime).
		Count(&count).Error

	if err != nil {
		return false, err
	}

	return count == 0, nil
}

func CreateAppointment(db *gorm.DB, phone, name string, startTime time.Time) error {
	appointment := models.Appointment{
		ClientPhone: phone,
		ClientName:  name,
		StartTime:   startTime,
		EndTime:     startTime.Add(AppointmentDuration),
		Status:      "CONFIRMED",
	}
	return db.Create(&appointment).Error
}
