# Borg Solder

The worker for the distributed task execution system.

## Features

- Connects to distributed task execution system via HTTP/gRPC
- Executes tasks (shell scripts, binaries, Docker containers)
- Downloads required files
- Uploads execution artifacts
- Sends heartbeats for health monitoring
- Screen monitoring (Windows, Linux, and macOS 12.3+)

## Setup

1. Install dependencies:
```bash
go mod download
```

**Note for macOS users:** Screen capture is supported on macOS 12.3+ using ScreenCaptureKit. You must grant Screen Recording permission in System Settings > Privacy & Security > Screen Recording. See `internal/screencapture/README_MACOS.md` for detailed setup instructions.

2. Run solder with command-line flags or config file:
```bash
# Using config file (recommended)
./solder --config config.yaml

# Or using command-line flags
./solder --mothership https://192.168.1.100:8080 --name my-runner --token my-token

# Or using environment variables
export MOTHERSHIP_ADDR=https://192.168.1.100:8080
export RUNNER_NAME=my-runner
export RUNNER_TOKEN=my-token
./solder
```

### Command-line Options

- `--mothership <address>` - Mothership server address (e.g., `https://ip:port` or `http://ip:port`)
- `--name <name>` - Runner name (defaults to hostname)
- `--token <token>` - Runner authentication token
- `--work-dir <path>` - Working directory for tasks (defaults to `./work`)
- `-h, --help` - Show help message

### Environment Variables (fallback if flags not provided)

- `MOTHERSHIP_ADDR` - Mothership server address (default: `http://localhost:8080`)
- `RUNNER_NAME` - Runner name (defaults to hostname)
- `RUNNER_TOKEN` - Runner authentication token (default: `default-token`)
- `WORK_DIR` - Working directory for tasks (default: `./work`)

**Priority:** Command-line flags > Environment variables > Default values

## Task Types

### Shell Script
Executes shell commands using the system shell (bash on Linux/Mac, PowerShell on Windows).

### Binary
Executes a binary file. The binary must be downloaded first and made executable.

### Docker
Executes a Docker container with the specified image and command.

