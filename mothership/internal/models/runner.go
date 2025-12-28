package models

import (
	"time"

	"gorm.io/gorm"
)

type Runner struct {
	ID               string    `gorm:"primaryKey;type:varchar(36)" json:"id"`
	DeviceID         string    `gorm:"uniqueIndex;type:varchar(64)" json:"device_id"` // Unique device identifier that persists through renames (SHA256 hash, 64 chars)
	Name             string    `gorm:"not null;type:varchar(255)" json:"name"`
	Hostname         string    `gorm:"not null;type:varchar(255)" json:"hostname"`
	OS               string    `gorm:"not null;type:varchar(100)" json:"os"`
	Architecture     string    `gorm:"not null;type:varchar(100)" json:"architecture"`
	MaxConcurrentTasks int32   `gorm:"default:1" json:"max_concurrent_tasks"`
	ActiveTasks      int32     `gorm:"default:0" json:"active_tasks"`
	Status           string    `gorm:"not null;type:varchar(50);default:'idle'" json:"status"`
	Labels           string    `gorm:"type:jsonb" json:"labels"` // JSON map stored as JSONB
	// Resource information
	CPUCores         int32     `gorm:"default:0" json:"cpu_cores"`
	CPUModel         string    `gorm:"type:varchar(255)" json:"cpu_model"`
	CPUFrequencyMHz  int32     `gorm:"default:0" json:"cpu_frequency_mhz"`
	MemoryGB         float64   `gorm:"default:0" json:"memory_gb"`
	DiskSpaceGB      float64   `gorm:"default:0" json:"disk_space_gb"` // Free/available disk space
	TotalDiskSpaceGB float64   `gorm:"default:0" json:"total_disk_space_gb"` // Total disk space
	OSVersion        string    `gorm:"type:varchar(100)" json:"os_version"`
	GPUInfo                string    `gorm:"type:text" json:"gpu_info"` // JSON array of GPU info
	PublicIPs              string    `gorm:"type:text" json:"public_ips"` // JSON array of IP addresses
	ScreenMonitoringEnabled bool     `gorm:"default:false" json:"screen_monitoring_enabled"`
	ScreenQuality          int32     `gorm:"default:60" json:"screen_quality"` // JPEG quality 1-100
	ScreenFPS              float64   `gorm:"default:2.0" json:"screen_fps"` // Frames per second (0.5-10)
	SelectedScreenIndex    int32     `gorm:"default:0" json:"selected_screen_index"` // Index of selected display (0 = primary)
	RegisteredAt           time.Time `gorm:"not null" json:"registered_at"`
	LastHeartbeat    time.Time `gorm:"not null" json:"last_heartbeat"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
	
	Tasks            []Task    `gorm:"foreignKey:RunnerID" json:"tasks,omitempty"`
}

func (Runner) TableName() string {
	return "runners"
}

