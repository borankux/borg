package models

import (
	"borg/mothership/internal/auth"
	"time"

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
	
	// Add runtimes column if it doesn't exist
	if !db.Migrator().HasColumn(&Runner{}, "runtimes") {
		if err := db.Migrator().AddColumn(&Runner{}, "runtimes"); err != nil {
			// Column might already exist, continue
		}
		// Set default value for existing runners (empty JSON array)
		db.Exec("UPDATE runners SET runtimes = '[]' WHERE runtimes IS NULL OR runtimes = ''")
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
	
	// Run full migration including User model
	if err := db.AutoMigrate(
		&User{},
		&Runner{},
		&Job{},
		&JobFile{},
		&Task{},
		&TaskLog{},
		&File{},
		&Artifact{},
		&ExecutorBinary{},
		&ProcessorScript{},
		&JobResult{},
	); err != nil {
		return err
	}
	
	// Create default user if no users exist
	var userCount int64
	db.Model(&User{}).Count(&userCount)
	
	if userCount == 0 {
		passwordHash, err := auth.HashPassword("mirzat")
		if err != nil {
			return err
		}
		
		defaultUser := &User{
			ID:           uuid.New().String(),
			Username:     "mirzat",
			PasswordHash: passwordHash,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		
		if err := db.Create(defaultUser).Error; err != nil {
			return err
		}
	}
	
	return nil
}

