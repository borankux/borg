package models

import (
	"time"

	"gorm.io/gorm"
)

type File struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name        string    `gorm:"not null;type:varchar(500)" json:"name"`
	Path        string    `gorm:"not null;type:varchar(1000)" json:"path"` // Storage path
	Size        int64     `gorm:"not null" json:"size"`
	ContentType string    `gorm:"type:varchar(255)" json:"content_type"`
	Hash        string    `gorm:"type:varchar(64);index" json:"hash"` // SHA256 hash
	UploadedBy  string    `gorm:"type:varchar(255)" json:"uploaded_by"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	
	JobFiles    []JobFile `gorm:"foreignKey:FileID" json:"job_files,omitempty"`
}

func (File) TableName() string {
	return "files"
}

// Artifact represents execution results uploaded by runners
type Artifact struct {
	ID          string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	TaskID      string    `gorm:"not null;type:varchar(36);index" json:"task_id"`
	Name        string    `gorm:"not null;type:varchar(500)" json:"name"`
	Path        string    `gorm:"not null;type:varchar(1000)" json:"path"` // Storage path
	Size        int64     `gorm:"not null" json:"size"`
	ContentType string    `gorm:"type:varchar(255)" json:"content_type"`
	Hash        string    `gorm:"type:varchar(64)" json:"hash"`
	CreatedAt   time.Time `gorm:"not null" json:"created_at"`
	
	Task        Task      `gorm:"foreignKey:TaskID" json:"task,omitempty"`
}

func (Artifact) TableName() string {
	return "artifacts"
}

