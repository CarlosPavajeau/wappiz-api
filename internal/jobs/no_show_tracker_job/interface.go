package no_show_tracker_job

import (
	"context"
	"wappiz/pkg/db"
	"wappiz/pkg/whatsapp"
)

type Config struct {
	DB            db.Database
	Whatsapp      whatsapp.Client
	EncryptionKey []byte
}

type NoShowTrackerJob interface {
	Run(ctx context.Context)
}
