package models

import (
	"gorm.io/gorm"
	"github.com/google/uuid"
)

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
	// First, add device_id column if it doesn't exist (nullable)
	if !db.Migrator().HasColumn(&Runner{}, "device_id") {
		if err := db.Migrator().AddColumn(&Runner{}, "device_id"); err != nil {
			// Column might already exist, continue
		}
	}
	
	// Backfill device_id for existing runners
	var runners []Runner
	db.Where("device_id IS NULL OR device_id = ''").Find(&runners)
	for i := range runners {
		if runners[i].DeviceID == "" {
			runners[i].DeviceID = uuid.New().String()
			db.Save(&runners[i])
		}
	}
	
	// Now run full migration
	return db.AutoMigrate(
		&Runner{},
		&Job{},
		&JobFile{},
		&Task{},
		&TaskLog{},
		&File{},
		&Artifact{},
	)
}

