package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"borg/mothership/internal/auth"
	"borg/mothership/internal/models"
	"borg/mothership/internal/queue"
	"borg/mothership/internal/storage"
	"borg/mothership/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Handler contains API handlers
type Handler struct {
	db        *gorm.DB
	queue     *queue.Queue
	storage   *storage.Storage
	screenHub *websocket.ScreenHub
}

// NewHandler creates a new API handler
func NewHandler(db *gorm.DB, q *queue.Queue, s *storage.Storage, screenHub *websocket.ScreenHub) *Handler {
	return &Handler{
		db:        db,
		queue:     q,
		storage:   s,
		screenHub: screenHub,
	}
}

// GetDashboardStats returns dashboard statistics
func (h *Handler) GetDashboardStats(c *gin.Context) {
	stats, err := h.queue.GetStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get runner count
	var runnerCount int64
	h.db.Model(&models.Runner{}).Count(&runnerCount)
	stats["runners"] = runnerCount

	c.JSON(http.StatusOK, stats)
}

// ListJobs returns a list of jobs
func (h *Handler) ListJobs(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	status := c.Query("status")

	jobs, total, err := h.queue.ListJobs(limit, offset, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetJob returns a single job
func (h *Handler) GetJob(c *gin.Context) {
	jobID := c.Param("id")

	job, err := h.queue.GetJob(jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, job)
}

// CreateJobRequest represents job creation request
type CreateJobRequest struct {
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Type             string          `json:"type"`
	Priority         int32           `json:"priority"`
	Command          string          `json:"command"`
	Args             json.RawMessage `json:"args"` // Can be array or null
	Env              json.RawMessage `json:"env"`  // Can be object or null
	WorkingDirectory string          `json:"working_directory"`
	TimeoutSeconds   int64           `json:"timeout_seconds"`
	MaxRetries       int32           `json:"max_retries"`
}

// CreateJob creates a new job
func (h *Handler) CreateJob(c *gin.Context) {
	var req CreateJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate required fields
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}

	// Set defaults
	if req.Type == "" {
		req.Type = "shell"
	}
	if req.Priority == 0 {
		req.Priority = 1
	}

	// Convert to Job model
	job := &models.Job{
		Name:             req.Name,
		Description:      req.Description,
		Type:             req.Type,
		Priority:         req.Priority,
		Command:          req.Command,
		WorkingDirectory: req.WorkingDirectory,
		TimeoutSeconds:   req.TimeoutSeconds,
		MaxRetries:       req.MaxRetries,
		Metadata:         "{}", // Initialize Metadata as empty JSON object
	}

	// Convert Args and Env to JSON strings - ensure valid JSON for PostgreSQL JSONB
	// Handle null, empty, or invalid JSON gracefully
	argsStr := strings.TrimSpace(string(req.Args))
	if len(argsStr) > 0 && argsStr != "null" {
		// Validate it's valid JSON array
		var argsArray []interface{}
		if err := json.Unmarshal(req.Args, &argsArray); err == nil {
			// Re-marshal to ensure proper formatting
			if argsJSON, err := json.Marshal(argsArray); err == nil {
				job.Args = string(argsJSON)
			} else {
				// Fallback to empty array if marshaling fails
				job.Args = "[]"
			}
		} else {
			// Invalid JSON - try to parse as any JSON value and wrap in array
			var singleArg interface{}
			if err := json.Unmarshal(req.Args, &singleArg); err == nil {
				if argsJSON, err := json.Marshal([]interface{}{singleArg}); err == nil {
					job.Args = string(argsJSON)
				} else {
					job.Args = "[]"
				}
			} else {
				// Completely invalid - use empty array
				job.Args = "[]"
			}
		}
	} else {
		// Empty or null - use empty array
		job.Args = "[]"
	}

	envStr := strings.TrimSpace(string(req.Env))
	if len(envStr) > 0 && envStr != "null" {
		// Validate it's valid JSON object
		var envMap map[string]interface{}
		if err := json.Unmarshal(req.Env, &envMap); err == nil {
			// Re-marshal to ensure proper formatting
			if envJSON, err := json.Marshal(envMap); err == nil {
				job.Env = string(envJSON)
			} else {
				// Fallback to empty object if marshaling fails
				job.Env = "{}"
			}
		} else {
			// Invalid JSON - use empty object
			job.Env = "{}"
		}
	} else {
		// Empty or null - use empty object
		job.Env = "{}"
	}

	if err := h.queue.EnqueueJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, job)
}

// PauseJob pauses a job
func (h *Handler) PauseJob(c *gin.Context) {
	jobID := c.Param("id")

	if err := h.queue.PauseJob(jobID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job paused"})
}

// ResumeJob resumes a job
func (h *Handler) ResumeJob(c *gin.Context) {
	jobID := c.Param("id")

	if err := h.queue.ResumeJob(jobID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job resumed"})
}

// CancelJob cancels a job
func (h *Handler) CancelJob(c *gin.Context) {
	jobID := c.Param("id")

	if err := h.queue.CancelJob(jobID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "job cancelled"})
}

// ListRunners returns a list of runners with calculated offline status
func (h *Handler) ListRunners(c *gin.Context) {
	var runners []models.Runner
	if err := h.db.Find(&runners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Calculate offline status based on last heartbeat (2 minutes threshold)
	now := time.Now()
	offlineThreshold := 2 * time.Minute

	for i := range runners {
		timeSinceHeartbeat := now.Sub(runners[i].LastHeartbeat)
		if timeSinceHeartbeat > offlineThreshold {
			// Override status to offline if heartbeat is too old
			runners[i].Status = "offline"
		}
	}

	c.JSON(http.StatusOK, runners)
}

// GetRunner returns a single runner
func (h *Handler) GetRunner(c *gin.Context) {
	runnerID := c.Param("id")

	var runner models.Runner
	if err := h.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "runner not found"})
		return
	}

	c.JSON(http.StatusOK, runner)
}

// RenameRunnerRequest represents runner rename request
type RenameRunnerRequest struct {
	Name string `json:"name" binding:"required"`
}

// RenameRunner renames a runner (DeviceID remains unchanged)
func (h *Handler) RenameRunner(c *gin.Context) {
	runnerID := c.Param("id")

	var req RenameRunnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	var runner models.Runner
	if err := h.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "runner not found"})
		return
	}

	// Update only the name, DeviceID remains unchanged
	runner.Name = req.Name
	runner.UpdatedAt = time.Now()

	if err := h.db.Save(&runner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "runner renamed successfully",
		"runner":  runner,
	})
}

// DeleteRunner deletes a runner
func (h *Handler) DeleteRunner(c *gin.Context) {
	runnerID := c.Param("id")

	var runner models.Runner
	if err := h.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "runner not found"})
		return
	}

	// Check if runner has active tasks
	if runner.ActiveTasks > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot delete runner with active tasks",
		})
		return
	}

	// Soft delete the runner
	if err := h.db.Delete(&runner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "runner deleted successfully",
	})
}

// GetTaskLogs returns logs for a task
func (h *Handler) GetTaskLogs(c *gin.Context) {
	taskID := c.Param("id")

	var logs []models.TaskLog
	if err := h.db.Where("task_id = ?", taskID).
		Order("timestamp ASC").
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, logs)
}

// Runner API endpoints

// RegisterRunnerRequest represents runner registration request
type RegisterRunnerRequest struct {
	Name               string            `json:"name"`
	Hostname           string            `json:"hostname"`
	DeviceID           string            `json:"device_id"`
	OS                 string            `json:"os"`
	Architecture       string            `json:"architecture"`
	MaxConcurrentTasks int32             `json:"max_concurrent_tasks"`
	Labels             map[string]string `json:"labels"`
	Token              string            `json:"token"`
	// Resource information
	CPUCores         int32     `json:"cpu_cores"`
	CPUModel         string    `json:"cpu_model"`
	CPUFrequencyMHz  int32     `json:"cpu_frequency_mhz"`
	MemoryGB         float64   `json:"memory_gb"`
	DiskSpaceGB      float64   `json:"disk_space_gb"`       // Free/available disk space
	TotalDiskSpaceGB float64   `json:"total_disk_space_gb"` // Total disk space
	OSVersion              string            `json:"os_version"`
	GPUInfo                []GPUInfo         `json:"gpu_info"`
	PublicIPs              []string          `json:"public_ips"`
	ScreenMonitoringEnabled bool             `json:"screen_monitoring_enabled"`
}

// GPUInfo represents GPU information
type GPUInfo struct {
	Name     string  `json:"name"`
	MemoryGB float64 `json:"memory_gb"`
	Driver   string  `json:"driver,omitempty"`
}

// RegisterRunnerResponse represents runner registration response
type RegisterRunnerResponse struct {
	RunnerID string `json:"runner_id"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// RegisterRunner registers a new runner or updates an existing one if the same device_id is found
// Falls back to hostname matching for backward compatibility if device_id is not provided
func (h *Handler) RegisterRunner(c *gin.Context) {
	var req RegisterRunnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Validate token
	// For now, accept any non-empty token
	if req.Token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "token required"})
		return
	}

	now := time.Now()
	labelsJSON, _ := json.Marshal(req.Labels)
	gpuInfoJSON, _ := json.Marshal(req.GPUInfo)
	publicIPsJSON, _ := json.Marshal(req.PublicIPs)

	// Check if a runner with the same device_id already exists (including soft-deleted)
	// If device_id is not provided, fall back to hostname for backward compatibility
	var existingRunner models.Runner
	var err error
	
	if req.DeviceID != "" {
		err = h.db.Unscoped().Where("device_id = ?", req.DeviceID).First(&existingRunner).Error
	} else {
		// Fallback to hostname for backward compatibility
		err = h.db.Unscoped().Where("hostname = ?", req.Hostname).First(&existingRunner).Error
	}

	if err == nil {
		// Runner exists - update it instead of creating a new one
		existingRunner.Name = req.Name
		existingRunner.Hostname = req.Hostname // Update hostname in case it changed
		existingRunner.OS = req.OS
		existingRunner.Architecture = req.Architecture
		existingRunner.MaxConcurrentTasks = req.MaxConcurrentTasks
		existingRunner.Status = "idle"
		existingRunner.Labels = string(labelsJSON)
		existingRunner.CPUCores = req.CPUCores
		existingRunner.CPUModel = req.CPUModel
		existingRunner.CPUFrequencyMHz = req.CPUFrequencyMHz
		existingRunner.MemoryGB = req.MemoryGB
		existingRunner.DiskSpaceGB = req.DiskSpaceGB
		existingRunner.TotalDiskSpaceGB = req.TotalDiskSpaceGB
		existingRunner.OSVersion = req.OSVersion
		existingRunner.GPUInfo = string(gpuInfoJSON)
		existingRunner.PublicIPs = string(publicIPsJSON)
		existingRunner.ScreenMonitoringEnabled = req.ScreenMonitoringEnabled
		existingRunner.LastHeartbeat = now
		existingRunner.UpdatedAt = now
		
		// Update device_id if provided and not already set
		if req.DeviceID != "" && existingRunner.DeviceID == "" {
			existingRunner.DeviceID = req.DeviceID
		}
		
		// If runner was soft-deleted, restore it by clearing DeletedAt
		if existingRunner.DeletedAt.Valid {
			existingRunner.DeletedAt = gorm.DeletedAt{}
			// Use Unscoped() to update the deleted_at field
			if err := h.db.Unscoped().Model(&existingRunner).Update("deleted_at", nil).Error; err != nil {
				c.JSON(http.StatusInternalServerError, RegisterRunnerResponse{
					Success: false,
					Message: err.Error(),
				})
				return
			}
		}

		if err := h.db.Save(&existingRunner).Error; err != nil {
			c.JSON(http.StatusInternalServerError, RegisterRunnerResponse{
				Success: false,
				Message: err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, RegisterRunnerResponse{
			RunnerID: existingRunner.ID,
			Success:  true,
			Message:  "runner re-registered successfully",
		})
		return
	}

	// Check if error is "not found" (expected) or a real database error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, RegisterRunnerResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	// Runner doesn't exist - create a new one
	runnerID := uuid.New().String()
	
	// Use provided device_id or generate a new one
	deviceID := req.DeviceID
	if deviceID == "" {
		deviceID = uuid.New().String() // Fallback: generate UUID if not provided
	}

	runner := &models.Runner{
		ID:                 runnerID,
		DeviceID:           deviceID,
		Name:               req.Name,
		Hostname:           req.Hostname,
		OS:                 req.OS,
		Architecture:       req.Architecture,
		MaxConcurrentTasks: req.MaxConcurrentTasks,
		Status:             "idle",
		Labels:             string(labelsJSON),
		CPUCores:           req.CPUCores,
		CPUModel:           req.CPUModel,
		CPUFrequencyMHz:    req.CPUFrequencyMHz,
		MemoryGB:           req.MemoryGB,
		DiskSpaceGB:        req.DiskSpaceGB,
		TotalDiskSpaceGB:   req.TotalDiskSpaceGB,
		OSVersion:          req.OSVersion,
		GPUInfo:                string(gpuInfoJSON),
		PublicIPs:              string(publicIPsJSON),
		ScreenMonitoringEnabled: req.ScreenMonitoringEnabled,
		RegisteredAt:           now,
		LastHeartbeat:          now,
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	if err := h.db.Create(runner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, RegisterRunnerResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, RegisterRunnerResponse{
		RunnerID: runnerID,
		Success:  true,
		Message:  "runner registered successfully",
	})
}

// HeartbeatRequest represents heartbeat request
type HeartbeatRequest struct {
	Status      string                 `json:"status"` // idle, busy, offline
	ActiveTasks int32                  `json:"active_tasks"`
	Resources   *ResourceUpdateRequest `json:"resources,omitempty"` // Optional resource update
}

// ResourceUpdateRequest represents resource information update
type ResourceUpdateRequest struct {
	DiskSpaceGB      float64  `json:"disk_space_gb,omitempty"`       // Free/available disk space
	TotalDiskSpaceGB float64  `json:"total_disk_space_gb,omitempty"` // Total disk space
	MemoryGB         float64  `json:"memory_gb,omitempty"`
	PublicIPs        []string `json:"public_ips,omitempty"`
}

// HeartbeatResponse represents heartbeat response
type HeartbeatResponse struct {
	Success               bool `json:"success"`
	NextHeartbeatInterval int  `json:"next_heartbeat_interval"` // seconds
}

// Heartbeat handles runner heartbeat
func (h *Handler) Heartbeat(c *gin.Context) {
	runnerID := c.Param("id")

	var req HeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now()
	updates := map[string]interface{}{
		"last_heartbeat": now,
		"status":         req.Status,
		"active_tasks":   req.ActiveTasks,
		"updated_at":     now,
	}

	// Update resource information if provided
	if req.Resources != nil {
		if req.Resources.DiskSpaceGB > 0 {
			updates["disk_space_gb"] = req.Resources.DiskSpaceGB
		}
		if req.Resources.TotalDiskSpaceGB > 0 {
			updates["total_disk_space_gb"] = req.Resources.TotalDiskSpaceGB
		}
		if req.Resources.MemoryGB > 0 {
			updates["memory_gb"] = req.Resources.MemoryGB
		}
		if len(req.Resources.PublicIPs) > 0 {
			publicIPsJSON, _ := json.Marshal(req.Resources.PublicIPs)
			updates["public_ips"] = string(publicIPsJSON)
		}
	}

	if err := h.db.Model(&models.Runner{}).Where("id = ?", runnerID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusNotFound, HeartbeatResponse{
			Success: false,
		})
		return
	}

	c.JSON(http.StatusOK, HeartbeatResponse{
		Success:               true,
		NextHeartbeatInterval: 30, // 30 seconds default
	})
}

// GetNextTaskResponse represents the next task for a runner
type GetNextTaskResponse struct {
	TaskID           string            `json:"task_id"`
	JobID            string            `json:"job_id"`
	JobName          string            `json:"job_name"`
	Type             string            `json:"type"` // shell, binary, docker
	Command          string            `json:"command"`
	Args             []string          `json:"args"`
	Env              map[string]string `json:"env"`
	WorkingDirectory string            `json:"working_directory"`
	TimeoutSeconds   int64             `json:"timeout_seconds"`
	DockerImage      string            `json:"docker_image"`
	Privileged       bool              `json:"privileged"`
	RequiredFiles    []string          `json:"required_files"`
}

// GetNextTask returns the next pending task for a runner
func (h *Handler) GetNextTask(c *gin.Context) {
	runnerID := c.Param("id")

	task, err := h.queue.GetNextTask(runnerID, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if task == nil {
		c.JSON(http.StatusOK, nil) // No tasks available
		return
	}

	// Load job
	var job models.Job
	if err := h.db.First(&job, "id = ?", task.JobID).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Load job files
	var jobFiles []models.JobFile
	h.db.Where("job_id = ?", job.ID).Find(&jobFiles)

	var args []string
	json.Unmarshal([]byte(job.Args), &args)

	var env map[string]string
	json.Unmarshal([]byte(job.Env), &env)
	if env == nil {
		env = make(map[string]string)
	}

	requiredFiles := make([]string, 0, len(jobFiles))
	for _, jf := range jobFiles {
		requiredFiles = append(requiredFiles, jf.FileID)
	}

	c.JSON(http.StatusOK, GetNextTaskResponse{
		TaskID:           task.ID,
		JobID:            job.ID,
		JobName:          job.Name,
		Type:             job.Type,
		Command:          job.Command,
		Args:             args,
		Env:              env,
		WorkingDirectory: job.WorkingDirectory,
		TimeoutSeconds:   job.TimeoutSeconds,
		DockerImage:      job.DockerImage,
		Privileged:       job.Privileged,
		RequiredFiles:    requiredFiles,
	})
}

// UpdateTaskStatusRequest represents task status update request
type UpdateTaskStatusRequest struct {
	Status       string `json:"status"` // pending, running, completed, failed, cancelled
	ExitCode     *int32 `json:"exit_code"`
	ErrorMessage string `json:"error_message"`
	Stdout       []byte `json:"stdout"`
	Stderr       []byte `json:"stderr"`
	Timestamp    int64  `json:"timestamp"`
}

// UpdateTaskStatusResponse represents task status update response
type UpdateTaskStatusResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// UpdateTaskStatus updates task status
func (h *Handler) UpdateTaskStatus(c *gin.Context) {
	taskID := c.Param("id")

	var req UpdateTaskStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exitCode := req.ExitCode
	if exitCode != nil && *exitCode == -1 {
		exitCode = nil
	}

	err := h.queue.UpdateTaskStatus(taskID, req.Status, exitCode, req.ErrorMessage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, UpdateTaskStatusResponse{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	// Store logs if provided
	if len(req.Stdout) > 0 || len(req.Stderr) > 0 {
		var taskLog models.TaskLog
		taskLog.TaskID = taskID
		taskLog.Timestamp = time.Unix(req.Timestamp, 0)
		if taskLog.Timestamp.IsZero() {
			taskLog.Timestamp = time.Now()
		}

		if len(req.Stdout) > 0 {
			taskLog.Level = "stdout"
			taskLog.Message = string(req.Stdout)
			h.db.Create(&taskLog)
		}

		if len(req.Stderr) > 0 {
			taskLog.Level = "stderr"
			taskLog.Message = string(req.Stderr)
			h.db.Create(&taskLog)
		}
	}

	c.JSON(http.StatusOK, UpdateTaskStatusResponse{
		Success: true,
		Message: "task status updated",
	})
}

// DownloadFile handles file download requests
func (h *Handler) DownloadFile(c *gin.Context) {
	fileID := c.Param("id")

	// Get file record
	var file models.File
	if err := h.db.First(&file, "id = ?", fileID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	// Open file
	fileReader, err := h.storage.GetFile(fileID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer fileReader.Close()

	// Set headers
	c.Header("Content-Disposition", `attachment; filename="`+file.Name+`"`)
	c.Header("Content-Type", file.ContentType)
	c.Header("Content-Length", strconv.FormatInt(file.Size, 10))

	// Stream file
	io.Copy(c.Writer, fileReader)
}

// UploadArtifact handles artifact upload from runner
func (h *Handler) UploadArtifact(c *gin.Context) {
	taskID := c.PostForm("task_id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id required"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Open uploaded file
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()

	// Save artifact
	artifactID, hash, size, err := h.storage.SaveArtifact(src, file.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Save artifact record
	artifact := &models.Artifact{
		ID:          artifactID,
		TaskID:      taskID,
		Name:        file.Filename,
		Path:        artifactID, // Storage path
		Size:        size,
		ContentType: file.Header.Get("Content-Type"),
		Hash:        hash,
		CreatedAt:   time.Now(),
	}

	if err := h.db.Create(artifact).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"artifact_id": artifactID,
		"success":     true,
		"message":     "artifact uploaded successfully",
	})
}

// UploadScreenshot handles screenshot upload from runner
func (h *Handler) UploadScreenshot(c *gin.Context) {
	runnerID := c.Param("id")

	var runner models.Runner
	if err := h.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "runner not found"})
		return
	}

	file, err := c.FormFile("screenshot")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()

	screenshotID := uuid.New().String()
	filename := fmt.Sprintf("%d_%s.jpg", time.Now().Unix(), screenshotID)
	path, err := h.storage.SaveScreenshot(runnerID, filename, src)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshot_id": screenshotID,
		"path":          path,
		"success":       true,
	})
}

// GetScreenshots returns list of screenshots for a runner
func (h *Handler) GetScreenshots(c *gin.Context) {
	runnerID := c.Param("id")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	screenshots, err := h.storage.ListScreenshots(runnerID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshots": screenshots,
	})
}

// GetScreenshot serves a specific screenshot
func (h *Handler) GetScreenshot(c *gin.Context) {
	runnerID := c.Param("id")
	filename := c.Param("filename")

	screenshotPath := h.storage.GetScreenshotPath(runnerID, filename)
	if screenshotPath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "screenshot not found"})
		return
	}

	c.File(screenshotPath)
}

// UploadScreenFrameRequest represents a screen frame upload request
type UploadScreenFrameRequest struct {
	Frame     string `json:"frame" binding:"required"` // base64 encoded JPEG
	Timestamp int64  `json:"timestamp"`
}

// UploadScreenFrame handles screen frame upload from agent
func (h *Handler) UploadScreenFrame(c *gin.Context) {
	runnerID := c.Param("id")
	
	if runnerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "runner ID is required"})
		return
	}

	var runner models.Runner
	if err := h.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "runner not found",
			"runner_id": runnerID,
		})
		return
	}

	var req UploadScreenFrameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate base64 frame (decode to check validity)
	_, err := base64.StdEncoding.DecodeString(req.Frame)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 frame data"})
		return
	}

	// Convert to base64 data URL format for frontend
	frameDataURL := "data:image/jpeg;base64," + req.Frame

	// Broadcast frame to all viewers
	h.screenHub.BroadcastFrame(runnerID, []byte(frameDataURL))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "frame received",
	})
}

// GetScreenStreamStatus returns the streaming status for a runner
func (h *Handler) GetScreenStreamStatus(c *gin.Context) {
	runnerID := c.Param("id")

	var runner models.Runner
	if err := h.db.First(&runner, "id = ?", runnerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "runner not found"})
		return
	}

	isStreaming := h.screenHub.IsStreaming(runnerID)
	viewerCount := h.screenHub.ViewerCount(runnerID)

	c.JSON(http.StatusOK, gin.H{
		"streaming":    isStreaming,
		"viewer_count": viewerCount,
	})
}

// DownloadSolder serves the solder executable for download
func (h *Handler) DownloadSolder(c *gin.Context) {
	// Try to find the solder binary in common locations
	possiblePaths := []string{
		"../solder/solder.exe", // Relative to mothership directory
		"./solder.exe",
		"/app/solder.exe",
		"solder.exe",
	}

	var filePath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			filePath = path
			break
		}
	}

	if filePath == "" {
		// Binary not found, return JSON message
		c.JSON(http.StatusNotFound, gin.H{
			"message":      "Solder binary not found. Please build from source.",
			"instructions": "See the Download page for build instructions.",
		})
		return
	}

	// Serve the binary file
	c.File(filePath)
}

// LoginRequest represents login request
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents login response
type LoginResponse struct {
	Token    string      `json:"token"`
	User     *models.User `json:"user"`
	Success  bool        `json:"success"`
	Message  string      `json:"message"`
}

// Login handles user authentication
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Find user by username
	var user models.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusUnauthorized, LoginResponse{
				Success: false,
				Message: "invalid username or password",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Verify password
	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, LoginResponse{
			Success: false,
			Message: "invalid username or password",
		})
		return
	}

	// Generate JWT token
	token, err := auth.GenerateJWT(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Return response (don't include password hash)
	userResponse := &models.User{
		ID:        user.ID,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}

	c.JSON(http.StatusOK, LoginResponse{
		Token:   token,
		User:   userResponse,
		Success: true,
		Message: "login successful",
	})
}

// GetCurrentUser returns the current authenticated user
func (h *Handler) GetCurrentUser(c *gin.Context) {
	// Get user from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var user models.User
	if err := h.db.First(&user, "id = ?", userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Return user without password hash
	userResponse := &models.User{
		ID:        user.ID,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}

	c.JSON(http.StatusOK, userResponse)
}
