package state_machine

import (
	"wappiz/internal/services/slot_finder"
	"wappiz/pkg/db"
	"wappiz/pkg/whatsapp"
)

type Config struct {
	DB          db.Database
	Whatsapp    whatsapp.Client
	SlotFinder  slot_finder.SlotFinderService
	Environment string
}

type service struct {
	db          db.Database
	whatsapp    whatsapp.Client
	slotFinder  slot_finder.SlotFinderService
	environment string
}

func New(cfg Config) *service {
	return &service{
		db:          cfg.DB,
		whatsapp:    cfg.Whatsapp,
		slotFinder:  cfg.SlotFinder,
		environment: cfg.Environment,
	}
}
