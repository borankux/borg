package executor

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Executor executes tasks
type Executor struct {
	workDir string
}

// NewExecutor creates a new executor
func NewExecutor(workDir string) (*Executor, error) {
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}
	
	return &Executor{
		workDir: workDir,
	}, nil
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
	
	switch job.Type {
	case "shell":
		cmd, err = e.executeShell(ctx, job, taskDir)
	case "binary":
		cmd, err = e.executeBinary(ctx, job, taskDir)
	case "docker":
		cmd, err = e.executeDocker(ctx, job, taskDir)
	default:
		return nil, fmt.Errorf("unsupported job type: %v", job.Type)
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

