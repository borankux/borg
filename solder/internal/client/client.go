package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Client handles HTTP communication with the mothership
type Client struct {
	baseURL    string
	httpClient *http.Client
	runnerID   string
}

// NewClient creates a new HTTP client connection to mothership
func NewClient(baseURL, runnerID string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		runnerID: runnerID,
	}
}

// Close closes the client (no-op for HTTP client, kept for compatibility)
func (c *Client) Close() error {
	return nil
}

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
	CPUModel         string            `json:"cpu_model"`
	CPUFrequencyMHz  int32             `json:"cpu_frequency_mhz"`
	MemoryGB         float64           `json:"memory_gb"`
	DiskSpaceGB      float64           `json:"disk_space_gb"` // Free/available disk space
	TotalDiskSpaceGB float64           `json:"total_disk_space_gb"` // Total disk space
	OSVersion        string            `json:"os_version"`
	GPUInfo          []GPUInfo         `json:"gpu_info"`
	PublicIPs        []string          `json:"public_ips"`
}

// GPUInfo represents GPU information
type GPUInfo struct {
	Name     string `json:"name"`
	MemoryGB float64 `json:"memory_gb"`
	Driver   string `json:"driver,omitempty"`
}

// RegisterRunnerResponse represents runner registration response
type RegisterRunnerResponse struct {
	RunnerID string `json:"runner_id"`
	Success  bool   `json:"success"`
	Message  string `json:"message"`
}

// RegisterRunner registers the runner with mothership
func (c *Client) RegisterRunner(ctx context.Context, req *RegisterRunnerRequest) (*RegisterRunnerResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/runners/register", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var registerResp RegisterRunnerResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &registerResp, nil
}

// Job represents a job from the API
type Job struct {
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

// GetNextTask gets the next task for the runner
func (c *Client) GetNextTask(ctx context.Context) (*Job, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/runners/"+c.runnerID+"/tasks/next", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check if no tasks available (null response)
	if len(bodyBytes) == 0 || string(bodyBytes) == "null" || string(bodyBytes) == "" {
		return nil, nil
	}

	var job Job
	if err := json.Unmarshal(bodyBytes, &job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &job, nil
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

// UpdateTaskStatusWithID updates task status with explicit task ID
func (c *Client) UpdateTaskStatusWithID(ctx context.Context, taskID string, req *UpdateTaskStatusRequest) (*UpdateTaskStatusResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/tasks/"+taskID+"/status", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var updateResp UpdateTaskStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&updateResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &updateResp, nil
}

// ResourceUpdate represents resource information update
type ResourceUpdate struct {
	DiskSpaceGB      float64  `json:"disk_space_gb,omitempty"`      // Free/available disk space
	TotalDiskSpaceGB float64  `json:"total_disk_space_gb,omitempty"` // Total disk space
	MemoryGB         float64  `json:"memory_gb,omitempty"`
	PublicIPs        []string `json:"public_ips,omitempty"`
}

// HeartbeatRequest represents heartbeat request
type HeartbeatRequest struct {
	Status      string         `json:"status"` // idle, busy, offline
	ActiveTasks int32          `json:"active_tasks"`
	Resources   *ResourceUpdate `json:"resources,omitempty"` // Optional resource update
}

// HeartbeatResponse represents heartbeat response
type HeartbeatResponse struct {
	Success             bool `json:"success"`
	NextHeartbeatInterval int `json:"next_heartbeat_interval"` // seconds
}

// Heartbeat sends heartbeat to mothership
func (c *Client) Heartbeat(ctx context.Context, status string, activeTasks int32, resources *ResourceUpdate) (*HeartbeatResponse, error) {
	req := HeartbeatRequest{
		Status:      status,
		ActiveTasks: activeTasks,
		Resources:   resources,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/runners/"+c.runnerID+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("heartbeat failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var heartbeatResp HeartbeatResponse
	if err := json.NewDecoder(resp.Body).Decode(&heartbeatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &heartbeatResp, nil
}

// DownloadFile downloads a file from mothership
func (c *Client) DownloadFile(ctx context.Context, fileID string, writer io.Writer) error {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/files/"+fileID+"/download", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if _, err := io.Copy(writer, resp.Body); err != nil {
		return fmt.Errorf("failed to copy file data: %w", err)
	}

	return nil
}

// UploadArtifactResponse represents artifact upload response
type UploadArtifactResponse struct {
	ArtifactID string `json:"artifact_id"`
	Success    bool   `json:"success"`
	Message    string `json:"message"`
}

// UploadArtifact uploads an artifact to mothership
func (c *Client) UploadArtifact(ctx context.Context, taskID, filename string, reader io.Reader) (*UploadArtifactResponse, error) {
	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add task_id field
	if err := writer.WriteField("task_id", taskID); err != nil {
		return nil, fmt.Errorf("failed to write task_id field: %w", err)
	}

	// Add file field
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create file field: %w", err)
	}

	if _, err := io.Copy(fileWriter, reader); err != nil {
		return nil, fmt.Errorf("failed to copy file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/artifacts/upload", &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var uploadResp UploadArtifactResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &uploadResp, nil
}

// StreamJobs polls for jobs and sends them to the channel
func (c *Client) StreamJobs(ctx context.Context, jobChan chan<- *Job, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Initial poll
	if job := c.pollForJob(ctx); job != nil {
		select {
		case jobChan <- job:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if job := c.pollForJob(ctx); job != nil {
				select {
				case jobChan <- job:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// pollForJob polls once for a job
func (c *Client) pollForJob(ctx context.Context) *Job {
	job, err := c.GetNextTask(ctx)
	if err != nil {
		// Log error but continue polling
		return nil
	}
	return job
}
