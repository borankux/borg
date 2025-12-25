# Borg - Distributed Task Execution System

The central orchestrator for the distributed task execution system.

## Features

- Job queue management with priority and retries
- Runner registration and health monitoring
- File storage and distribution
- REST API for web dashboard
- WebSocket for real-time updates
- gRPC service for solder communication

## Setup

1. Install dependencies:
```bash
go mod download
```

2. Set up PostgreSQL database (using docker-compose):
```bash
docker-compose up -d postgres
```

3. Set environment variables (create `.env` file):
```
DATABASE_URL=host=localhost user=postgres password=postgres dbname=borg port=5432 sslmode=disable
STORAGE_PATH=./storage
HTTP_PORT=8080
GRPC_PORT=50051
```

4. Run migrations and start server:
```bash
go run cmd/server/main.go
```

## API Endpoints

- `GET /api/v1/stats` - Dashboard statistics
- `GET /api/v1/jobs` - List jobs
- `POST /api/v1/jobs` - Create job
- `GET /api/v1/jobs/:id` - Get job details
- `POST /api/v1/jobs/:id/pause` - Pause job
- `POST /api/v1/jobs/:id/resume` - Resume job
- `POST /api/v1/jobs/:id/cancel` - Cancel job
- `GET /api/v1/runners` - List runners
- `GET /api/v1/runners/:id` - Get runner details
- `GET /api/v1/tasks/:id/logs` - Get task logs
- `WS /ws` - WebSocket endpoint for real-time updates

## Web Frontend

The web frontend is located in the `web/` directory. To develop:

```bash
cd web
npm install
npm run dev
```

To build for production:

```bash
cd web
npm install
npm run build
```

The built files will be in `web/dist/` and served by the Go server.

