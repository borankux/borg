package api

import (
	"encoding/json"
	"io"
	"net/http"
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
		"jobs":  jobs,
		"total": total,
		"limit": limit,
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

// CreateJob creates a new job
func (h *Handler) CreateJob(c *gin.Context) {
	var job models.Job
	if err := c.ShouldBindJSON(&job); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Set defaults
	if job.Type == "" {
		job.Type = "shell"
	}
	if job.Priority == 0 {
		job.Priority = 1
	}
	
	if err := h.queue.EnqueueJob(&job); err != nil {
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

// ListRunners returns a list of runners
func (h *Handler) ListRunners(c *gin.Context) {
	var runners []models.Runner
	if err := h.db.Find(&runners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
		"runner": runner,
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
	Name             string            `json:"name"`
	Hostname         string            `json:"hostname"`
	OS               string            `json:"os"`
	Architecture     string            `json:"architecture"`
	MaxConcurrentTasks int32           `json:"max_concurrent_tasks"`
	Labels           map[string]string `json:"labels"`
	Token            string            `json:"token"`
	// Resource information
	CPUCores         int32             `json:"cpu_cores"`
	MemoryGB         float64           `json:"memory_gb"`
	DiskSpaceGB      float64           `json:"disk_space_gb"`
	GPUInfo          []GPUInfo         `json:"gpu_info"`
	PublicIPs        []string          `json:"public_ips"`
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
		ID:               runnerID,
		DeviceID:         deviceID,
		Name:             req.Name,
		Hostname:         req.Hostname,
		OS:               req.OS,
		Architecture:     req.Architecture,
		MaxConcurrentTasks: req.MaxConcurrentTasks,
		Status:           "idle",
		Labels:           string(labelsJSON),
		CPUCores:         req.CPUCores,
		MemoryGB:         req.MemoryGB,
		DiskSpaceGB:      req.DiskSpaceGB,
		GPUInfo:          string(gpuInfoJSON),
		PublicIPs:        string(publicIPsJSON),
		RegisteredAt:     now,
		LastHeartbeat:    now,
		CreatedAt:        now,
		UpdatedAt:        now,
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
	Status      string `json:"status"` // idle, busy, offline
	ActiveTasks int32  `json:"active_tasks"`
}

// HeartbeatResponse represents heartbeat response
type HeartbeatResponse struct {
	Success             bool  `json:"success"`
	NextHeartbeatInterval int `json:"next_heartbeat_interval"` // seconds
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
	
	if err := h.db.Model(&models.Runner{}).Where("id = ?", runnerID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusNotFound, HeartbeatResponse{
			Success: false,
		})
		return
	}
	
	c.JSON(http.StatusOK, HeartbeatResponse{
		Success:              true,
		NextHeartbeatInterval: 30, // 30 seconds default
	})
}

// GetNextTaskResponse represents the next task for a runner
type GetNextTaskResponse struct {
	TaskID    string   `json:"task_id"`
	JobID     string   `json:"job_id"`
	JobName   string   `json:"job_name"`
	Type      string   `json:"type"` // shell, binary, docker
	Command   string   `json:"command"`
	Args      []string `json:"args"`
	Env       map[string]string `json:"env"`
	WorkingDirectory string `json:"working_directory"`
	TimeoutSeconds  int64  `json:"timeout_seconds"`
	DockerImage     string `json:"docker_image"`
	Privileged      bool   `json:"privileged"`
	RequiredFiles   []string `json:"required_files"`
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
	Status      string `json:"status"` // pending, running, completed, failed, cancelled
	ExitCode    *int32 `json:"exit_code"`
	ErrorMessage string `json:"error_message"`
	Stdout      []byte `json:"stdout"`
	Stderr      []byte `json:"stderr"`
	Timestamp   int64  `json:"timestamp"`
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

