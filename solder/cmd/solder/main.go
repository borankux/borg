package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"borg/solder/internal/client"
	"borg/solder/internal/config"
	"borg/solder/internal/deviceid"
	"borg/solder/internal/downloader"
	"borg/solder/internal/executor"
	"borg/solder/internal/heartbeat"
	"borg/solder/internal/resources"
	"borg/solder/internal/screencapture"
	"borg/solder/internal/uploader"
)

func main() {
	// Set custom usage function
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment variables (used as fallback if flags not provided):\n")
		fmt.Fprintf(os.Stderr, "  MOTHERSHIP_ADDR  - Mothership server address\n")
		fmt.Fprintf(os.Stderr, "  RUNNER_NAME      - Runner name\n")
		fmt.Fprintf(os.Stderr, "  RUNNER_TOKEN     - Runner authentication token\n")
		fmt.Fprintf(os.Stderr, "  WORK_DIR         - Working directory for tasks\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s --config config.yaml\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --mothership https://192.168.1.100:8080 --name my-runner\n", os.Args[0])
	}

	// Command-line flags
	var configPath = flag.String("config", "", "Path to config file (YAML)")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Determine runner name
	runnerName := cfg.Solder.Name
	if runnerName == "" {
		hostname, _ := os.Hostname()
		runnerName = hostname
	}

	// Generate or get cached device ID
	deviceID, err := deviceid.GetOrGenerateDeviceID(cfg.Work.Directory)
	if err != nil {
		log.Fatalf("Failed to generate device ID: %v", err)
	}
	log.Printf("Device ID: %s", deviceID)

	// Create screen capture service to check availability
	screenCapture := screencapture.NewCaptureService(cfg.ScreenCapture)
	screenMonitoringEnabled := screenCapture.IsEnabled()

	// Log permission status on macOS
	if runtime.GOOS == "darwin" {
		if screenMonitoringEnabled {
			log.Println("✅ Screen Recording permission granted - screen monitoring enabled")
		} else if cfg.ScreenCapture.Enabled {
			log.Printf(`
⚠️  Screen Recording Permission Required

Screen monitoring is disabled because Screen Recording permission is not granted.

To enable screen monitoring:
1. Open System Settings > Privacy & Security > Screen Recording
2. Enable the checkbox for: %s
3. Restart the agent

The agent will continue running without screen monitoring.
`, os.Args[0])
		}
	}

	// Create client
	httpClient := client.NewClient(cfg.Server.Address, "")

	// Convert runtime configs to client format
	runtimeConfigs := make([]client.RuntimeConfig, 0, len(cfg.Runtimes))
	for _, rt := range cfg.Runtimes {
		runtimeConfigs = append(runtimeConfigs, client.RuntimeConfig{
			Name: rt.Name,
			Path: rt.Path,
			URL:  rt.URL,
		})
	}

	// Register runner
	ctx := context.Background()
	log.Printf("Registering to mothership: %s", cfg.Server.Address)

	registerReq := &client.RegisterRunnerRequest{
		Name:                    runnerName,
		Hostname:                getHostname(),
		DeviceID:                deviceID,
		OS:                      runtime.GOOS,
		Architecture:            runtime.GOARCH,
		MaxConcurrentTasks:      cfg.Tasks.MaxConcurrent,
		Labels:                  getLabels(),
		Token:                   cfg.Solder.Token,
		ScreenMonitoringEnabled: screenMonitoringEnabled,
		Runtimes:               runtimeConfigs,
	}

	// Detect and fill resource information
	log.Println("Detecting system resources...")
	if err := resources.FillResources(ctx, registerReq); err != nil {
		log.Printf("Warning: Failed to detect some resources: %v", err)
	}
	log.Printf("Resources detected - CPU: %d cores, Memory: %.2f GB, Disk: %.2f GB, GPU: %d, Public IPs: %v",
		registerReq.CPUCores, registerReq.MemoryGB, registerReq.DiskSpaceGB, len(registerReq.GPUInfo), registerReq.PublicIPs)

	registerResp, err := httpClient.RegisterRunner(ctx, registerReq)
	if err != nil {
		log.Fatalf("Registration failed to mothership %s: %v", cfg.Server.Address, err)
	}

	if !registerResp.Success {
		log.Fatalf("Registration to mothership %s failed: %s", cfg.Server.Address, registerResp.Message)
	}

	runnerID := registerResp.RunnerID
	log.Printf("✅ Successfully registered to mothership %s with runner ID: %s", cfg.Server.Address, runnerID)

	// Recreate client with runner ID
	httpClient = client.NewClient(cfg.Server.Address, runnerID)

	// Attempt to connect WebSocket for real-time communication
	log.Printf("Attempting to connect WebSocket to mothership...")
	if err := httpClient.ConnectAgentWebSocket(ctx); err != nil {
		log.Printf("⚠️  WebSocket connection failed: %v. Falling back to HTTP polling.", err)
	} else {
		log.Printf("✅ WebSocket connected successfully. Using real-time communication.")
	}

	// Create executor
	exec, err := executor.NewExecutor(cfg.Work.Directory)
	if err != nil {
		log.Fatalf("Failed to create executor: %v", err)
	}

	// Convert runtime configs to executor format and set them
	executorRuntimes := make([]executor.RuntimeConfig, 0, len(cfg.Runtimes))
	for _, rt := range cfg.Runtimes {
		executorRuntimes = append(executorRuntimes, executor.RuntimeConfig{
			Name: rt.Name,
			Path: rt.Path,
			URL:  rt.URL,
		})
	}
	exec.SetRuntimes(executorRuntimes)
	if len(executorRuntimes) > 0 {
		log.Printf("Configured %d runtime(s)", len(executorRuntimes))
	}

	// Create downloader
	dl := downloader.NewDownloader(httpClient, cfg.Work.Directory)

	// Create uploader
	up := uploader.NewUploader(httpClient)

	// Create heartbeat with config interval
	hb := heartbeat.NewHeartbeat(httpClient, runnerID, time.Duration(cfg.Heartbeat.IntervalSeconds)*time.Second)

	// Context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Screen streaming management
	var streamingCtx context.Context
	var streamingCancel context.CancelFunc
	var streamingMu sync.Mutex

	// Start screen streaming monitor (only if capture is enabled)
	if screenCapture.IsEnabled() {
		go func() {
			ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					streamingMu.Lock()
					if streamingCancel != nil {
						streamingCancel()
						streamingCancel = nil
					}
					streamingMu.Unlock()
					return
				case <-ticker.C:
					// Check if streaming is requested and get settings
					status, err := httpClient.GetScreenStreamStatus(ctx)
					if err != nil {
						log.Printf("Failed to check screen stream status: %v", err)
						continue
					}

					streamingMu.Lock()
					isStreaming := streamingCancel != nil
					
					// Get current settings to check if they changed
					currentQuality := screenCapture.GetQuality()
					currentInterval := screenCapture.GetInterval()
					desiredInterval := time.Duration(float64(time.Second) / status.FPS)
					settingsChanged := currentQuality != int(status.Quality) || 
						(currentInterval != desiredInterval && desiredInterval >= 100*time.Millisecond && desiredInterval <= 2*time.Second)

					// Update screen capture settings
					screenCapture.UpdateSettings(int(status.Quality), status.FPS)
					
					// Update screen index if changed
					if err := screenCapture.SetScreenIndex(int(status.ScreenIndex)); err != nil {
						log.Printf("Failed to set screen index %d: %v", status.ScreenIndex, err)
					}
					
					// If streaming and settings changed significantly, restart streaming
					if isStreaming && settingsChanged {
						log.Printf("Screen settings changed (quality=%d, fps=%.1f), restarting stream", 
							status.Quality, status.FPS)
						streamingCancel()
						streamingCancel = nil
						screenCapture.StopStreaming()
						isStreaming = false // Reset flag so we restart below
					}

					if status.Streaming && !isStreaming {
						// Start streaming with current settings
						log.Printf("Starting screen streaming (viewers: %d, quality=%d, fps=%.1f)", 
							status.ViewerCount, status.Quality, status.FPS)

						// Try to connect WebSocket for faster streaming
						if err := httpClient.ConnectScreenWebSocket(ctx); err != nil {
							log.Printf("Failed to connect WebSocket, falling back to HTTP: %v", err)
						} else {
							log.Println("WebSocket connected for screen streaming")
						}

						streamingCtx, streamingCancel = context.WithCancel(ctx)
						go screenCapture.StartStreaming(streamingCtx, func(data []byte) error {
							// Send frame synchronously - the StartStreaming function now handles backpressure
							// with its internal queue, so we can send directly here
							if err := httpClient.SendScreenFrameBinary(streamingCtx, data); err != nil {
								log.Printf("Failed to send frame: %v", err)
								return err
							}
							return nil
						})
					} else if !status.Streaming && isStreaming {
						// Stop streaming
						log.Println("Stopping screen streaming (no viewers)")
						streamingCancel()
						streamingCancel = nil
						screenCapture.StopStreaming()
						httpClient.CloseScreenWebSocket()
					}
					streamingMu.Unlock()
				}
			}
		}()
		log.Println("Screen streaming monitor enabled")
	} else {
		log.Println("Screen capture not available (disabled or no desktop environment)")
	}

	// Start heartbeat
	go hb.Start(ctx)

	// Task management
	var activeTasks sync.Map
	var taskCounter int32

	// Job streaming (WebSocket with HTTP fallback)
	jobChan := make(chan *client.Job, 10)
	go func() {
		useWebSocket := httpClient.IsAgentWebSocketConnected()
		for {
			var err error
			if useWebSocket {
				// Try WebSocket first
				err = httpClient.StreamJobsWebSocket(ctx, jobChan)
				if err != nil {
					log.Printf("WebSocket job stream error: %v. Falling back to HTTP polling.", err)
					useWebSocket = false
					// Continue to HTTP fallback
				}
			}

			if !useWebSocket {
				// HTTP polling fallback
				if err := httpClient.StreamJobs(ctx, jobChan, 5*time.Second); err != nil {
					log.Printf("HTTP job stream error: %v", err)
					time.Sleep(5 * time.Second)
				}
				// Try WebSocket again after a delay
				time.Sleep(30 * time.Second)
				if httpClient.IsAgentWebSocketConnected() {
					log.Printf("WebSocket reconnected, switching to real-time communication")
					useWebSocket = true
				}
			}
		}
	}()

	// Process jobs
	semaphore := make(chan struct{}, cfg.Tasks.MaxConcurrent)

	go func() {
		for job := range jobChan {
			// Check if we have capacity
			select {
			case semaphore <- struct{}{}:
				// Start task
				go func(j *client.Job) {
					defer func() { <-semaphore }()

					taskID := j.TaskID
					taskNum := atomic.AddInt32(&taskCounter, 1)
					taskDir := filepath.Join(cfg.Work.Directory, fmt.Sprintf("task_%s_%d", taskID, taskNum))

					activeTasks.Store(taskID, true)
					hb.SetActiveTasks(int32(len(semaphore)))

					log.Printf("Starting task %s: %s", taskID, j.JobName)

					// Download required files
					for i, fileID := range j.RequiredFiles {
						destPath := filepath.Join(taskDir, fmt.Sprintf("file_%d", i))
						if err := dl.DownloadFile(ctx, fileID, destPath); err != nil {
							log.Printf("Failed to download file %s: %v", fileID, err)
							failReq := &client.UpdateTaskStatusRequest{
								Status:       "failed",
								ErrorMessage: fmt.Sprintf("failed to download file: %v", err),
								Timestamp:    time.Now().Unix(),
							}
							if err := httpClient.SendTaskStatusWebSocket(ctx, taskID, failReq); err != nil {
								httpClient.UpdateTaskStatusWithID(ctx, taskID, failReq)
							}
							activeTasks.Delete(taskID)
							hb.SetActiveTasks(int32(len(semaphore)))
							return
						}
					}

					// Notify task started (WebSocket with HTTP fallback)
					statusReq := &client.UpdateTaskStatusRequest{
						Status:    "running",
						Timestamp: time.Now().Unix(),
					}
					if err := httpClient.SendTaskStatusWebSocket(ctx, taskID, statusReq); err != nil {
						httpClient.UpdateTaskStatusWithID(ctx, taskID, statusReq)
					}

					// Create stdout/stderr buffers
					stdoutBuf := make([]byte, 0)
					stderrBuf := make([]byte, 0)

					// Create multi-writers to capture output
					stdoutWriter := &bufferWriter{
						buf:    &stdoutBuf,
						client: httpClient,
						taskID: taskID,
						ctx:    ctx,
					}
					stderrWriter := &bufferWriter{
						buf:    &stderrBuf,
						client: httpClient,
						taskID: taskID,
						ctx:    ctx,
						isErr:  true,
					}

					// Convert client.Job to executor.Job
					execJob := &executor.Job{
						TaskID:           j.TaskID,
						JobID:            j.JobID,
						JobName:          j.JobName,
						Type:             j.Type,
						Command:          j.Command,
						Args:             j.Args,
						Env:              j.Env,
						WorkingDirectory: j.WorkingDirectory,
						TimeoutSeconds:   j.TimeoutSeconds,
						DockerImage:      j.DockerImage,
						Privileged:       j.Privileged,
						RequiredFiles:    j.RequiredFiles,
						ExecutorBinaryID: j.ExecutorBinaryID,
						TaskData:         j.TaskData,
					}

					// Execute task
					result, err := exec.Execute(ctx, execJob, taskDir, stdoutWriter, stderrWriter)

					status := "completed"
					exitCode := result.ExitCode
					errorMsg := ""

					if err != nil || exitCode != 0 {
						status = "failed"
						if err != nil {
							errorMsg = err.Error()
						}
					}

					// For executor_binary type, upload results
					if j.Type == "executor_binary" {
						// Check for result.json file
						resultJSONPath := filepath.Join(taskDir, "result.json")
						if resultData, err := os.ReadFile(resultJSONPath); err == nil {
							// Upload result
							if err := httpClient.UploadJobResult(ctx, j.TaskID, j.JobID, string(resultData), taskDir); err != nil {
								log.Printf("Failed to upload job result: %v", err)
							}
						} else {
							// If no result.json, create one from stdout if available
							if len(stdoutBuf) > 0 {
								resultData := map[string]interface{}{
									"stdout": string(stdoutBuf),
									"stderr": string(stderrBuf),
									"exit_code": exitCode,
								}
								resultJSON, _ := json.Marshal(resultData)
								if err := httpClient.UploadJobResult(ctx, j.TaskID, j.JobID, string(resultJSON), taskDir); err != nil {
									log.Printf("Failed to upload job result: %v", err)
								}
							}
						}
					}

					// Upload artifacts if any
					artifactDir := filepath.Join(taskDir, "artifacts")
					if _, err := os.Stat(artifactDir); err == nil {
						_, err := up.UploadArtifacts(ctx, taskID, artifactDir)
						if err != nil {
							log.Printf("Failed to upload artifacts: %v", err)
						}
					}

					// Send final status (WebSocket with HTTP fallback)
					finalStatusReq := &client.UpdateTaskStatusRequest{
						Status:       status,
						ExitCode:     &exitCode,
						ErrorMessage: errorMsg,
						Stdout:       stdoutBuf,
						Stderr:       stderrBuf,
						Timestamp:    time.Now().Unix(),
					}
					if err := httpClient.SendTaskStatusWebSocket(ctx, taskID, finalStatusReq); err != nil {
						httpClient.UpdateTaskStatusWithID(ctx, taskID, finalStatusReq)
					}

					log.Printf("Task %s completed with status %s", taskID, status)
					activeTasks.Delete(taskID)
					hb.SetActiveTasks(int32(len(semaphore)))
				}(job)
			default:
				log.Printf("Max concurrent tasks reached, skipping job")
			}
		}
	}()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Println("Solder agent running. Press Ctrl+C to stop.")
	<-sigChan

	log.Println("Shutting down...")
	cancel()
	hb.Stop()

	// Wait for active tasks to complete (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			log.Println("Timeout waiting for tasks to complete")
			return
		case <-ticker.C:
			count := 0
			activeTasks.Range(func(key, value interface{}) bool {
				count++
				return true
			})
			if count == 0 {
				log.Println("All tasks completed")
				return
			}
			log.Printf("Waiting for %d tasks to complete...", count)
		}
	}
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func getLabels() map[string]string {
	labels := make(map[string]string)
	labels["os"] = runtime.GOOS
	labels["arch"] = runtime.GOARCH
	return labels
}

// bufferWriter captures output and sends it to mothership
type bufferWriter struct {
	buf    *[]byte
	client *client.Client
	taskID string
	ctx    context.Context
	isErr  bool
}

func (w *bufferWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)

	// Send output update (WebSocket with HTTP fallback)
	req := &client.UpdateTaskStatusRequest{
		Status:    "running",
		Timestamp: time.Now().Unix(),
	}
	if w.isErr {
		req.Stderr = p
	} else {
		req.Stdout = p
	}

	// Don't block on status updates
	go func() {
		if err := w.client.SendTaskStatusWebSocket(w.ctx, w.taskID, req); err != nil {
			w.client.UpdateTaskStatusWithID(w.ctx, w.taskID, req)
		}
	}()

	return len(p), nil
}
