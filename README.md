# Borg - Distributed Task Execution System

A distributed task execution system where a central orchestrator manages job queues and distributes tasks to solder runners across different devices.

## Architecture

- **Distributed Task Execution System**: Central orchestrator managing job queues, file distribution, runner registration, and web dashboard
- **Solder**: Worker agents that execute tasks and report results

## Quick Start

### 1. Start PostgreSQL

```bash
docker-compose up -d postgres
```

### 2. Start Distributed Task Execution System

```bash
cd mothership
go mod download
go run cmd/server/main.go
```

The distributed task execution system will start on:
- HTTP API: http://localhost:8080
- gRPC: localhost:50051
- WebSocket: ws://localhost:8080/ws

### 3. Build and Start Web Frontend

```bash
cd mothership/web
npm install
npm run dev
```

Or build for production:

```bash
cd mothership/web
npm install
npm run build
```

### 4. Start Solder Runner

```bash
cd solder
go mod download
export MOTHERSHIP_ADDR=localhost:50051
go run cmd/agent/main.go
```

## Components

### Distributed Task Execution System (`mothership/`)

- Backend API (Go)
- PostgreSQL database
- gRPC server for solder communication
- REST API for web dashboard
- WebSocket for real-time updates
- File storage service

### Solder (`solder/`)

- gRPC client for distributed task execution system communication
- Task executor (shell, binary, Docker)
- File downloader/uploader
- Heartbeat mechanism

### Web Dashboard (`mothership/web/`)

- React + TypeScript frontend
- Tailwind CSS with glass morphism effects
- Real-time updates via WebSocket
- Bento-grid layout

## Features

- Job management (create, pause, resume, cancel)
- Priority-based job scheduling
- Automatic retries with exponential backoff
- Runner health monitoring via heartbeat
- File distribution and artifact collection
- Real-time dashboard with WebSocket updates
- Support for shell scripts, binaries, and Docker containers

## Development

See individual README files in `mothership/` and `solder/` directories for more details.

## License

MIT

