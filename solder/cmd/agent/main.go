package main

import (
	"context"
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
	runnerName := cfg.Agent.Name
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

	// Create client
	httpClient := client.NewClient(cfg.Server.Address, "")

	// Register runner
	ctx := context.Background()
	registerReq := &client.RegisterRunnerRequest{
		Name:                   runnerName,
		Hostname:               getHostname(),
		DeviceID:               deviceID,
		OS:                     runtime.GOOS,
		Architecture:           runtime.GOARCH,
		MaxConcurrentTasks:     cfg.Tasks.MaxConcurrent,
		Labels:                 getLabels(),
		Token:                  cfg.Agent.Token,
		ScreenMonitoringEnabled: screenMonitoringEnabled,
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
		log.Fatalf("Failed to register runner: %v", err)
	}

	if !registerResp.Success {
		log.Fatalf("Runner registration failed: %s", registerResp.Message)
	}

	runnerID := registerResp.RunnerID
	log.Printf("Registered runner: %s", runnerID)

	// Recreate client with runner ID
	httpClient = client.NewClient(cfg.Server.Address, runnerID)

	// Create executor
	exec, err := executor.NewExecutor(cfg.Work.Directory)
	if err != nil {
		log.Fatalf("Failed to create executor: %v", err)
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

	// Start screen capture service
	if screenCapture.IsEnabled() {
		go screenCapture.Start(ctx, func(data []byte) error {
			return httpClient.UploadScreenshot(ctx, data)
		})
		log.Println("Screen capture enabled")
	} else {
		log.Println("Screen capture not available (disabled or no desktop environment)")
	}

	// Start heartbeat
	go hb.Start(ctx)

	// Task management
	var activeTasks sync.Map
	var taskCounter int32

	// Job polling
	jobChan := make(chan *client.Job, 10)
	go func() {
		for {
			if err := httpClient.StreamJobs(ctx, jobChan, 5*time.Second); err != nil {
				log.Printf("Job stream error: %v", err)
				time.Sleep(5 * time.Second)
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
							httpClient.UpdateTaskStatusWithID(ctx, taskID, &client.UpdateTaskStatusRequest{
								Status:       "failed",
								ErrorMessage: fmt.Sprintf("failed to download file: %v", err),
								Timestamp:    time.Now().Unix(),
							})
							activeTasks.Delete(taskID)
							hb.SetActiveTasks(int32(len(semaphore)))
							return
						}
					}

					// Notify task started
					httpClient.UpdateTaskStatusWithID(ctx, taskID, &client.UpdateTaskStatusRequest{
						Status:    "running",
						Timestamp: time.Now().Unix(),
					})

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

					// Upload artifacts if any
					artifactDir := filepath.Join(taskDir, "artifacts")
					if _, err := os.Stat(artifactDir); err == nil {
						_, err := up.UploadArtifacts(ctx, taskID, artifactDir)
						if err != nil {
							log.Printf("Failed to upload artifacts: %v", err)
						}
					}

					// Send final status
					httpClient.UpdateTaskStatusWithID(ctx, taskID, &client.UpdateTaskStatusRequest{
						Status:       status,
						ExitCode:     &exitCode,
						ErrorMessage: errorMsg,
						Stdout:       stdoutBuf,
						Stderr:       stderrBuf,
						Timestamp:    time.Now().Unix(),
					})

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

	// Send output update
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
	go w.client.UpdateTaskStatusWithID(w.ctx, w.taskID, req)

	return len(p), nil
}
