package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// RuntimeConfig represents a runtime configuration
type RuntimeConfig struct {
	Name string
	Path string
	URL  string
}

// Executor executes tasks
type Executor struct {
	workDir   string
	runtimes  map[string]RuntimeConfig // runtime name -> config
	runtimeMu sync.RWMutex
}

// NewExecutor creates a new executor
func NewExecutor(workDir string) (*Executor, error) {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}
	
	return &Executor{
		workDir:  workDir,
		runtimes: make(map[string]RuntimeConfig),
	}, nil
}

// SetRuntimes configures the runtimes available for execution
func (e *Executor) SetRuntimes(runtimes []RuntimeConfig) {
	e.runtimeMu.Lock()
	defer e.runtimeMu.Unlock()

	// Clear existing runtimes
	e.runtimes = make(map[string]RuntimeConfig)

	// Add new runtimes
	for _, rt := range runtimes {
		if rt.Name != "" {
			e.runtimes[rt.Name] = rt
		}
	}
}

// ExecuteResult contains execution results
type ExecuteResult struct {
	ExitCode    int32
	Stdout      []byte
	Stderr      []byte
	Error       error
}

// Execute executes a job
func (e *Executor) Execute(ctx context.Context, job *Job, taskDir string, stdout, stderr io.Writer) (*ExecuteResult, error) {
	// Create task directory
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create task directory: %w", err)
	}
	
	var cmd *exec.Cmd
	var err error
	
	// Check if job.Type is a configured runtime
	e.runtimeMu.RLock()
	runtimeConfig, isRuntime := e.runtimes[job.Type]
	e.runtimeMu.RUnlock()

	if isRuntime {
		cmd, err = e.executeRuntime(ctx, job, taskDir, runtimeConfig)
	} else {
		// Use existing job types
		switch job.Type {
		case "shell":
			cmd, err = e.executeShell(ctx, job, taskDir)
		case "binary":
			cmd, err = e.executeBinary(ctx, job, taskDir)
		case "docker":
			cmd, err = e.executeDocker(ctx, job, taskDir)
		case "executor_binary":
			cmd, err = e.executeExecutorBinary(ctx, job, taskDir)
		default:
			return nil, fmt.Errorf("unsupported job type: %v", job.Type)
		}
	}
	
	if err != nil {
		return nil, err
	}
	
	// Set working directory
	if job.WorkingDirectory != "" {
		cmd.Dir = filepath.Join(taskDir, job.WorkingDirectory)
	} else {
		cmd.Dir = taskDir
	}
	
	// Set environment variables
	env := os.Environ()
	for k, v := range job.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env
	
	// Capture stdout and stderr
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	
	// Set timeout if specified
	if job.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(job.TimeoutSeconds)*time.Second)
		defer cancel()
	}
	
	// Execute command
	err = cmd.Run()
	exitCode := int32(0)
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitError.ExitCode())
		} else {
			// Context timeout or other error
			exitCode = -1
		}
	}
	
	return &ExecuteResult{
		ExitCode: exitCode,
		Error:    err,
	}, nil
}

// executeShell executes a shell command
func (e *Executor) executeShell(ctx context.Context, job *Job, taskDir string) (*exec.Cmd, error) {
	var shell string
	var shellArgs []string
	
	switch runtime.GOOS {
	case "windows":
		shell = "powershell"
		shellArgs = []string{"-Command", job.Command}
	default:
		shell = "/bin/sh"
		shellArgs = []string{"-c", job.Command}
	}
	
	cmd := exec.CommandContext(ctx, shell, shellArgs...)
	return cmd, nil
}

// executeBinary executes a binary
func (e *Executor) executeBinary(ctx context.Context, job *Job, taskDir string) (*exec.Cmd, error) {
	// Command should be the path to the binary
	binaryPath := filepath.Join(taskDir, job.Command)
	
	// Check if binary exists
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("binary not found: %s", binaryPath)
	}
	
	// Make binary executable (Unix)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binaryPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to make binary executable: %w", err)
		}
	}
	
	cmd := exec.CommandContext(ctx, binaryPath, job.Args...)
	return cmd, nil
}

// executeDocker executes a Docker container
func (e *Executor) executeDocker(ctx context.Context, job *Job, taskDir string) (*exec.Cmd, error) {
	// Check if docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, fmt.Errorf("docker not found: %w", err)
	}
	
	if job.DockerImage == "" {
		return nil, fmt.Errorf("docker image not specified")
	}
	
	// Build docker run command
	args := []string{"run", "--rm"}
	
	// Mount task directory
	args = append(args, "-v", fmt.Sprintf("%s:/work", taskDir))
	args = append(args, "-w", "/work")
	
	if job.WorkingDirectory != "" {
		args = append(args, "-w", filepath.Join("/work", job.WorkingDirectory))
	}
	
	// Environment variables
	for k, v := range job.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	
	// Privileged mode
	if job.Privileged {
		args = append(args, "--privileged")
	}
	
	// Image and command
	args = append(args, job.DockerImage)
	args = append(args, job.Command)
	args = append(args, job.Args...)
	
	cmd := exec.CommandContext(ctx, "docker", args...)
	return cmd, nil
}

// executeExecutorBinary executes an executor binary with task data
func (e *Executor) executeExecutorBinary(ctx context.Context, job *Job, taskDir string) (*exec.Cmd, error) {
	// Find executor binary in required files (should be the last one)
	var binaryPath string
	for i := len(job.RequiredFiles) - 1; i >= 0; i-- {
		// Check if this file is the executor binary
		// The executor binary should be in taskDir as "file_N" where N is the index
		candidatePath := filepath.Join(taskDir, fmt.Sprintf("file_%d", i))
		if info, err := os.Stat(candidatePath); err == nil && !info.IsDir() {
			// Check if it's executable (or make it executable)
			if runtime.GOOS != "windows" {
				os.Chmod(candidatePath, 0755)
			}
			binaryPath = candidatePath
			break
		}
	}

	if binaryPath == "" {
		// Try to find by executor binary ID in filename
		if job.ExecutorBinaryID != "" {
			// Look for file with executor binary ID in name
			files, err := os.ReadDir(taskDir)
			if err == nil {
				for _, file := range files {
					if !file.IsDir() {
						potentialPath := filepath.Join(taskDir, file.Name())
						if runtime.GOOS != "windows" {
							os.Chmod(potentialPath, 0755)
						}
						binaryPath = potentialPath
						break
					}
				}
			}
		}
	}

	if binaryPath == "" {
		return nil, fmt.Errorf("executor binary not found in task directory")
	}

	// Write TaskData as JSON file and set environment variable
	env := os.Environ()
	if job.TaskData != nil && len(job.TaskData) > 0 {
		taskDataJSON, err := json.Marshal(job.TaskData)
		if err == nil {
			// Write to file
			taskDataPath := filepath.Join(taskDir, "task_data.json")
			os.WriteFile(taskDataPath, taskDataJSON, 0644)

			// Set environment variable
			env = append(env, fmt.Sprintf("TASK_DATA_JSON=%s", string(taskDataJSON)))
		}
	}

	// Set other environment variables
	for k, v := range job.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute binary
	cmd := exec.CommandContext(ctx, binaryPath, job.Args...)
	cmd.Env = env
	return cmd, nil
}

// executeRuntime executes a job using a configured runtime
func (e *Executor) executeRuntime(ctx context.Context, job *Job, taskDir string, runtimeConfig RuntimeConfig) (*exec.Cmd, error) {
	var executablePath string
	var err error

	// Determine executable path
	if runtimeConfig.URL != "" {
		// Download runtime from URL if needed
		executablePath, err = e.downloadRuntime(ctx, runtimeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to download runtime %s: %w", runtimeConfig.Name, err)
		}
	} else if runtimeConfig.Path != "" {
		// Use provided path
		executablePath = runtimeConfig.Path
		// Verify the executable exists
		if _, err := os.Stat(executablePath); os.IsNotExist(err) {
			return nil, fmt.Errorf("runtime executable not found: %s", executablePath)
		}
	} else {
		return nil, fmt.Errorf("runtime %s has no path or URL configured", runtimeConfig.Name)
	}

	// Build command: runtimePath command [args...]
	// The command becomes the first argument to the runtime, and job.Args become additional arguments
	args := make([]string, 0, 1+len(job.Args))
	args = append(args, job.Command)
	args = append(args, job.Args...)

	cmd := exec.CommandContext(ctx, executablePath, args...)
	return cmd, nil
}

// downloadRuntime downloads a runtime binary from URL and caches it
func (e *Executor) downloadRuntime(ctx context.Context, runtimeConfig RuntimeConfig) (string, error) {
	// Create runtime cache directory
	runtimeCacheDir := filepath.Join(e.workDir, ".runtimes")
	if err := os.MkdirAll(runtimeCacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create runtime cache directory: %w", err)
	}

	// Cache path: work/.runtimes/{runtime-name}
	cachedPath := filepath.Join(runtimeCacheDir, runtimeConfig.Name)

	// Check if already cached
	if info, err := os.Stat(cachedPath); err == nil && !info.IsDir() {
		// Runtime is cached, return cached path
		return cachedPath, nil
	}

	// Download runtime
	req, err := http.NewRequestWithContext(ctx, "GET", runtimeConfig.URL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download runtime: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download runtime: HTTP %d", resp.StatusCode)
	}

	// Create cached file
	file, err := os.Create(cachedPath)
	if err != nil {
		return "", fmt.Errorf("failed to create cached runtime file: %w", err)
	}
	defer file.Close()

	// Copy downloaded data to file
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(cachedPath) // Clean up on error
		return "", fmt.Errorf("failed to write cached runtime file: %w", err)
	}

	// Make executable on Unix systems
	if runtime.GOOS != "windows" {
		if err := os.Chmod(cachedPath, 0755); err != nil {
			os.Remove(cachedPath) // Clean up on error
			return "", fmt.Errorf("failed to make runtime executable: %w", err)
		}
	}

	return cachedPath, nil
}

