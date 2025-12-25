package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"borg/mothership/internal/models"
	"borg/mothership/internal/queue"
	"borg/mothership/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Handler contains API handlers
type Handler struct {
	db      *gorm.DB
	queue   *queue.Queue
	storage *storage.Storage
}

// NewHandler creates a new API handler
func NewHandler(db *gorm.DB, q *queue.Queue, s *storage.Storage) *Handler {
	return &Handler{
		db:      db,
		queue:   q,
		storage: s,
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
	}

	// Convert Args and Env to JSON strings
	if len(req.Args) > 0 {
		// Validate it's valid JSON
		var argsArray []interface{}
		if err := json.Unmarshal(req.Args, &argsArray); err == nil {
			// Re-marshal to ensure proper formatting
			if argsJSON, err := json.Marshal(argsArray); err == nil {
				job.Args = string(argsJSON)
			} else {
				job.Args = string(req.Args)
			}
		} else {
			job.Args = string(req.Args)
		}
	} else {
		job.Args = "[]"
	}

	if len(req.Env) > 0 {
		// Validate it's valid JSON
		var envMap map[string]interface{}
		if err := json.Unmarshal(req.Env, &envMap); err == nil {
			// Re-marshal to ensure proper formatting
			if envJSON, err := json.Marshal(envMap); err == nil {
				job.Env = string(envJSON)
			} else {
				job.Env = string(req.Env)
			}
		} else {
			job.Env = string(req.Env)
		}
	} else {
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
	OSVersion        string    `json:"os_version"`
	GPUInfo          []GPUInfo `json:"gpu_info"`
	PublicIPs        []string  `json:"public_ips"`
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

// RegisterRunner registers a new runner
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

	runnerID := uuid.New().String()
	deviceID := uuid.New().String() // Unique device ID that persists through renames
	now := time.Now()

	labelsJSON, _ := json.Marshal(req.Labels)
	gpuInfoJSON, _ := json.Marshal(req.GPUInfo)
	publicIPsJSON, _ := json.Marshal(req.PublicIPs)

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
		GPUInfo:            string(gpuInfoJSON),
		PublicIPs:          string(publicIPsJSON),
		RegisteredAt:       now,
		LastHeartbeat:      now,
		CreatedAt:          now,
		UpdatedAt:          now,
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
