package models

import (
	"time"

	"gorm.io/gorm"
)

type Task struct {
	ID            string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	JobID         string    `gorm:"not null;type:varchar(36);index" json:"job_id"`
	RunnerID      string    `gorm:"type:varchar(36);index" json:"runner_id"`
	Status        string    `gorm:"not null;type:varchar(50);default:'pending';index" json:"status"`
	StartedAt     *time.Time `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	ExitCode      *int32     `json:"exit_code"`
	ErrorMessage  string     `gorm:"type:text" json:"error_message"`
	TaskData      string     `gorm:"type:jsonb" json:"task_data"` // CSV row data as JSON
	RetryCount    int32      `gorm:"default:0" json:"retry_count"`
	IsDispatched  bool       `gorm:"default:false;index" json:"is_dispatched"` // Whether task has been dispatched to a runner
	Result        string     `gorm:"type:jsonb" json:"result"`                  // JSON result data from processing
	Reason        string     `gorm:"type:text" json:"reason"`                  // Failure reason when status is failed
	CreatedAt     time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
	
	Job           Job       `gorm:"foreignKey:JobID" json:"job,omitempty"`
	Runner        Runner    `gorm:"foreignKey:RunnerID" json:"runner,omitempty"`
	Logs          []TaskLog `gorm:"foreignKey:TaskID" json:"logs,omitempty"`
	JobResults    []JobResult `gorm:"foreignKey:TaskID" json:"job_results,omitempty"`
}

func (Task) TableName() string {
	return "tasks"
}

// TaskLog stores execution logs for tasks
type TaskLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TaskID    string    `gorm:"not null;type:varchar(36);index" json:"task_id"`
	Level     string    `gorm:"not null;type:varchar(20);default:'info'" json:"level"` // stdout, stderr, info, error
	Message   string    `gorm:"type:text" json:"message"`
	Timestamp time.Time `gorm:"not null;index" json:"timestamp"`
	
	Task      Task      `gorm:"foreignKey:TaskID" json:"task,omitempty"`
}

func (TaskLog) TableName() string {
	return "task_logs"
}

