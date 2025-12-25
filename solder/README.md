# Borg Solder

The worker agent for the distributed task execution system.

## Features

- Connects to distributed task execution system via HTTP/gRPC
- Executes tasks (shell scripts, binaries, Docker containers)
- Downloads required files
- Uploads execution artifacts
- Sends heartbeats for health monitoring

## Setup

1. Install dependencies:
```bash
go mod download
```

2. Set environment variables:
```
MOTHERSHIP_ADDR=localhost:50051
RUNNER_NAME=my-runner
RUNNER_TOKEN=default-token
WORK_DIR=./work
```

3. Run the agent:
```bash
go run cmd/agent/main.go
```

## Task Types

### Shell Script
Executes shell commands using the system shell (bash on Linux/Mac, PowerShell on Windows).

### Binary
Executes a binary file. The binary must be downloaded first and made executable.

### Docker
Executes a Docker container with the specified image and command.

