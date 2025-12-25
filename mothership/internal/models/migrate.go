package models

import (
	"gorm.io/gorm"
)

// Migrate runs database migrations
func Migrate(db *gorm.DB) error {
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

