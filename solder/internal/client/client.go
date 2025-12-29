package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client handles HTTP and WebSocket communication with the mothership
type Client struct {
	baseURL    string
	httpClient *http.Client
	runnerID   string

	// WebSocket connection for screen streaming
	screenWSConn   *websocket.Conn
	screenWSMu     sync.Mutex
	screenWSDialer *websocket.Dialer

	// WebSocket connection for agent communication
	agentWSClient *AgentWebSocketClient
	agentWSMu     sync.Mutex
}

// NewClient creates a new HTTP client connection to mothership
func NewClient(baseURL, runnerID string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		runnerID: runnerID,
		screenWSDialer: &websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		},
	}
}

// SetRunnerID sets the runner ID for the client
func (c *Client) SetRunnerID(runnerID string) {
	c.runnerID = runnerID
}

// Close closes the client (closes WebSocket connections if open)
func (c *Client) Close() error {
	c.agentWSMu.Lock()
	if c.agentWSClient != nil {
		c.agentWSClient.Disconnect()
		c.agentWSClient = nil
	}
	c.agentWSMu.Unlock()
	return c.CloseScreenWebSocket()
}

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
	CPUCores                int32             `json:"cpu_cores"`
	CPUModel                string            `json:"cpu_model"`
	CPUFrequencyMHz         int32             `json:"cpu_frequency_mhz"`
	MemoryGB                float64           `json:"memory_gb"`
	DiskSpaceGB             float64           `json:"disk_space_gb"`       // Free/available disk space
	TotalDiskSpaceGB        float64           `json:"total_disk_space_gb"` // Total disk space
	OSVersion               string            `json:"os_version"`
	GPUInfo                 []GPUInfo         `json:"gpu_info"`
	PublicIPs               []string          `json:"public_ips"`
	ScreenMonitoringEnabled bool              `json:"screen_monitoring_enabled"`
	Runtimes                []RuntimeConfig   `json:"runtimes"`
}

// GPUInfo represents GPU information
type GPUInfo struct {
	Name     string  `json:"name"`
	MemoryGB float64 `json:"memory_gb"`
	Driver   string  `json:"driver,omitempty"`
}

// RuntimeConfig represents a runtime configuration
type RuntimeConfig struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
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
	TaskID           string                 `json:"task_id"`
	JobID            string                 `json:"job_id"`
	JobName          string                 `json:"job_name"`
	Type             string                 `json:"type"` // shell, binary, docker, executor_binary
	Command          string                 `json:"command"`
	Args             []string               `json:"args"`
	Env              map[string]string      `json:"env"`
	WorkingDirectory string                 `json:"working_directory"`
	TimeoutSeconds   int64                  `json:"timeout_seconds"`
	DockerImage      string                 `json:"docker_image"`
	Privileged       bool                   `json:"privileged"`
	RequiredFiles    []string               `json:"required_files"`
	ExecutorBinaryID string                `json:"executor_binary_id,omitempty"` // For executor_binary type
	TaskData         map[string]interface{} `json:"task_data,omitempty"`          // CSV row data
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
	DiskSpaceGB      float64  `json:"disk_space_gb,omitempty"`       // Free/available disk space
	TotalDiskSpaceGB float64  `json:"total_disk_space_gb,omitempty"` // Total disk space
	MemoryGB         float64  `json:"memory_gb,omitempty"`
	PublicIPs        []string `json:"public_ips,omitempty"`
}

// HeartbeatRequest represents heartbeat request
type HeartbeatRequest struct {
	Status      string          `json:"status"` // idle, busy, offline
	ActiveTasks int32           `json:"active_tasks"`
	Resources   *ResourceUpdate `json:"resources,omitempty"` // Optional resource update
}

// HeartbeatResponse represents heartbeat response
type HeartbeatResponse struct {
	Success               bool `json:"success"`
	NextHeartbeatInterval int  `json:"next_heartbeat_interval"` // seconds
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

// UploadScreenshot uploads a screenshot to mothership (deprecated - use SendScreenFrame instead)
func (c *Client) UploadScreenshot(ctx context.Context, screenshotData []byte) error {
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	fileWriter, err := writer.CreateFormFile("screenshot", "screenshot.jpg")
	if err != nil {
		return fmt.Errorf("failed to create file field: %w", err)
	}

	if _, err := fileWriter.Write(screenshotData); err != nil {
		return fmt.Errorf("failed to write screenshot data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v1/runners/"+c.runnerID+"/screenshots", &requestBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// SendScreenFrame sends a screen frame to mothership for streaming
func (c *Client) SendScreenFrame(ctx context.Context, frameData []byte) error {
	if c.runnerID == "" {
		return fmt.Errorf("runner ID not set, cannot send screen frame")
	}

	// Encode frame as base64
	frameBase64 := base64.StdEncoding.EncodeToString(frameData)

	req := map[string]interface{}{
		"frame":     frameBase64,
		"timestamp": time.Now().Unix(),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		c.baseURL+"/api/v1/runners/"+c.runnerID+"/screen/frame", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("frame upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// ScreenStreamStatus represents screen stream status and settings
type ScreenStreamStatus struct {
	Streaming   bool    `json:"streaming"`
	ViewerCount int     `json:"viewer_count"`
	Quality     int32   `json:"quality"`
	FPS         float64 `json:"fps"`
	ScreenIndex int32   `json:"screen_index"` // Index of selected screen (0 = primary)
}

// GetScreenStreamStatus checks if screen streaming is requested and returns settings
func (c *Client) GetScreenStreamStatus(ctx context.Context) (*ScreenStreamStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET",
		c.baseURL+"/api/v1/runners/"+c.runnerID+"/screen/status", nil)
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

	var statusResp ScreenStreamStatus
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Set defaults if not provided
	if statusResp.Quality == 0 {
		statusResp.Quality = 60
	}
	if statusResp.FPS == 0 {
		statusResp.FPS = 2.0
	}

	return &statusResp, nil
}

// ConnectScreenWebSocket connects to the screen streaming WebSocket endpoint
func (c *Client) ConnectScreenWebSocket(ctx context.Context) error {
	c.screenWSMu.Lock()
	defer c.screenWSMu.Unlock()

	if c.screenWSConn != nil {
		// Already connected
		return nil
	}

	if c.runnerID == "" {
		return fmt.Errorf("runner ID not set, cannot connect WebSocket")
	}

	// Convert HTTP URL to WebSocket URL
	wsURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}

	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = fmt.Sprintf("/ws/screen/agent/%s", c.runnerID)

	conn, _, err := c.screenWSDialer.DialContext(ctx, wsURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	c.screenWSConn = conn
	return nil
}

// SendScreenFrameBinary sends a screen frame via WebSocket as binary data
func (c *Client) SendScreenFrameBinary(ctx context.Context, frameData []byte) error {
	c.screenWSMu.Lock()
	conn := c.screenWSConn
	c.screenWSMu.Unlock()

	if conn == nil {
		// Fallback to HTTP if WebSocket not connected
		return c.SendScreenFrame(ctx, frameData)
	}

	// Set write deadline
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	// Send binary frame
	err := conn.WriteMessage(websocket.BinaryMessage, frameData)
	if err != nil {
		// Connection lost, close and fallback to HTTP
		c.screenWSMu.Lock()
		if c.screenWSConn == conn {
			c.screenWSConn.Close()
			c.screenWSConn = nil
		}
		c.screenWSMu.Unlock()

		// Fallback to HTTP
		return c.SendScreenFrame(ctx, frameData)
	}

	return nil
}

// CloseScreenWebSocket closes the screen streaming WebSocket connection
func (c *Client) CloseScreenWebSocket() error {
	c.screenWSMu.Lock()
	defer c.screenWSMu.Unlock()

	if c.screenWSConn != nil {
		err := c.screenWSConn.Close()
		c.screenWSConn = nil
		return err
	}
	return nil
}

<<<<<<< HEAD
// ConnectAgentWebSocket connects to the agent WebSocket endpoint
func (c *Client) ConnectAgentWebSocket(ctx context.Context) error {
	c.agentWSMu.Lock()
	defer c.agentWSMu.Unlock()

	if c.agentWSClient != nil && c.agentWSClient.IsConnected() {
		return nil // Already connected
	}

	if c.runnerID == "" {
		return fmt.Errorf("runner ID not set, cannot connect WebSocket")
	}

	c.agentWSClient = NewAgentWebSocketClient(c.baseURL, c.runnerID)
	return c.agentWSClient.Connect(ctx)
}

// StreamJobsWebSocket streams jobs via WebSocket
func (c *Client) StreamJobsWebSocket(ctx context.Context, jobChan chan<- *Job) error {
	c.agentWSMu.Lock()
	client := c.agentWSClient
	c.agentWSMu.Unlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	messageChan := client.ReceiveMessages()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-messageChan:
			if !ok {
				return fmt.Errorf("WebSocket message channel closed")
			}

			if message.Type == "task_assignment" {
				var job Job
				if err := json.Unmarshal(message.Data, &job); err != nil {
					// Log error but continue - will be handled by HTTP fallback
					continue
				}

				select {
				case jobChan <- &job:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// UploadJobResult uploads a job result with JSON data and optional files
func (c *Client) UploadJobResult(ctx context.Context, taskID, jobID, resultDataJSON string, taskDir string) error {
	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Add fields
	writer.WriteField("task_id", taskID)
	writer.WriteField("job_id", jobID)
	writer.WriteField("result_data", resultDataJSON)

	// Add files from task directory (look for result files)
	if taskDir != "" {
		// Look for files in task directory that might be results
		files, err := os.ReadDir(taskDir)
		if err == nil {
			for _, file := range files {
				if !file.IsDir() && file.Name() != "task_data.json" {
					filePath := filepath.Join(taskDir, file.Name())
					fileReader, err := os.Open(filePath)
					if err == nil {
						fileWriter, err := writer.CreateFormFile("files[]", file.Name())
						if err == nil {
							io.Copy(fileWriter, fileReader)
						}
						fileReader.Close()
					}
				}
			}
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/v1/jobs/"+jobID+"/results/upload", &requestBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// SendHeartbeatWebSocket sends a heartbeat via WebSocket
func (c *Client) SendHeartbeatWebSocket(ctx context.Context, status string, activeTasks int32, resources *ResourceUpdate) error {
	c.agentWSMu.Lock()
	client := c.agentWSClient
	c.agentWSMu.Unlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	heartbeatData := map[string]interface{}{
		"status":       status,
		"active_tasks": activeTasks,
	}

	if resources != nil {
		heartbeatData["resources"] = resources
	}

	return client.SendMessage("heartbeat", heartbeatData)
}

// SendTaskStatusWebSocket sends a task status update via WebSocket
func (c *Client) SendTaskStatusWebSocket(ctx context.Context, taskID string, req *UpdateTaskStatusRequest) error {
	c.agentWSMu.Lock()
	client := c.agentWSClient
	c.agentWSMu.Unlock()

	if client == nil || !client.IsConnected() {
		return fmt.Errorf("WebSocket not connected")
	}

	statusData := map[string]interface{}{
		"task_id":   taskID,
		"status":    req.Status,
		"timestamp": req.Timestamp,
	}

	if req.ExitCode != nil {
		statusData["exit_code"] = *req.ExitCode
	}
	if req.ErrorMessage != "" {
		statusData["error_message"] = req.ErrorMessage
	}
	if len(req.Stdout) > 0 {
		statusData["stdout"] = req.Stdout
	}
	if len(req.Stderr) > 0 {
		statusData["stderr"] = req.Stderr
	}

	return client.SendMessage("task_status", statusData)
}

// IsAgentWebSocketConnected returns whether the agent WebSocket is connected
func (c *Client) IsAgentWebSocketConnected() bool {
	c.agentWSMu.Lock()
	defer c.agentWSMu.Unlock()
	return c.agentWSClient != nil && c.agentWSClient.IsConnected()
}

