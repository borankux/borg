package models

import (
	"time"

	"gorm.io/gorm"
)

type Job struct {
	ID              string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	Name            string    `gorm:"not null;type:varchar(255)" json:"name"`
	Description     string    `gorm:"type:text" json:"description"`
	Type            string    `gorm:"not null;type:varchar(50)" json:"type"` // shell, binary, docker
	Priority        int32     `gorm:"default:1" json:"priority"` // 0=low, 1=normal, 2=high, 3=urgent
	Command         string    `gorm:"not null;type:text" json:"command"`
	Args            string    `gorm:"type:jsonb" json:"args"` // JSON array
	Env             string    `gorm:"type:jsonb" json:"env"` // JSON map
	WorkingDirectory string   `gorm:"type:varchar(500)" json:"working_directory"`
	TimeoutSeconds  int64     `gorm:"default:0" json:"timeout_seconds"`
	MaxRetries      int32     `gorm:"default:0" json:"max_retries"`
	DockerImage     string    `gorm:"type:varchar(500)" json:"docker_image"`
	Privileged      bool      `gorm:"default:false" json:"privileged"`
	Metadata        string    `gorm:"type:jsonb" json:"metadata"` // JSON map
	ExecutorBinaryID string   `gorm:"type:varchar(36);index" json:"executor_binary_id"` // Reusable executor binary
	ProcessorScriptID string   `gorm:"type:varchar(36);index" json:"processor_script_id"` // Processor script for this job
	CSVDatasetID    string    `gorm:"type:varchar(36);index" json:"csv_dataset_id"` // CSV dataset file
	Status          string    `gorm:"not null;type:varchar(50);default:'pending'" json:"status"`
	CreatedBy       string    `gorm:"type:varchar(255)" json:"created_by"`
	CreatedAt       time.Time `gorm:"not null" json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	
	Tasks           []Task    `gorm:"foreignKey:JobID" json:"tasks,omitempty"`
	JobFiles        []JobFile `gorm:"foreignKey:JobID" json:"job_files,omitempty"`
	JobResults      []JobResult `gorm:"foreignKey:JobID" json:"job_results,omitempty"`
}

func (Job) TableName() string {
	return "jobs"
}

// JobFile represents files required for a job
type JobFile struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	JobID     string    `gorm:"not null;type:varchar(36);index" json:"job_id"`
	FileID    string    `gorm:"not null;type:varchar(36);index" json:"file_id"`
	Path      string    `gorm:"not null;type:varchar(500)" json:"path"` // Destination path on runner
	CreatedAt time.Time `json:"created_at"`
	
	Job       Job       `gorm:"foreignKey:JobID" json:"job,omitempty"`
	File      File      `gorm:"foreignKey:FileID" json:"file,omitempty"`
}

func (JobFile) TableName() string {
	return "job_files"
}

