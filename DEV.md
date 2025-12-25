# Development Guide

This guide explains how to run the backend and frontend locally for faster development without rebuilding Docker containers.

## Prerequisites

1. **PostgreSQL in Docker** - Database must be running
2. **Go** - For backend development
3. **Node.js & npm** - For frontend development

## Quick Start

### Option 1: Run Everything (Recommended)

```powershell
# Start PostgreSQL (if not already running)
docker-compose up -d postgres

# Run both backend and frontend
.\run-dev.ps1
```

This will:
- Start the backend server on `http://localhost:8080`
- Start the frontend dev server on `http://localhost:5173`
- Open each in a separate PowerShell window

### Option 2: Run Separately

**Backend only:**
```powershell
cd mothership
.\run-backend.ps1
```

**Frontend only:**
```powershell
cd mothership/web
.\run-frontend.ps1
```

## Development URLs

- **Frontend**: http://localhost:5173 (Vite dev server with hot reload)
- **Backend API**: http://localhost:8080/api/v1
- **Backend Web**: http://localhost:8080 (serves built frontend in production)

## Environment Variables

The backend uses these defaults (can be overridden with `.env` file):

- `DATABASE_URL`: `host=localhost user=postgres password=postgres dbname=borg port=5432 sslmode=disable`
- `STORAGE_PATH`: `./storage` (relative to mothership directory)
- `HTTP_PORT`: `8080`

## Hot Reload

- **Frontend**: Vite automatically reloads on file changes
- **Backend**: Restart the Go server manually (Ctrl+C and run again)

## Stopping Services

- Close the PowerShell windows running backend/frontend
- Or press `Ctrl+C` in each window

## Database

Only PostgreSQL runs in Docker. To manage it:

```powershell
# Start database
docker-compose up -d postgres

# Stop database
docker-compose stop postgres

# View database logs
docker-compose logs -f postgres

# Connect to database
docker-compose exec postgres psql -U postgres -d borg
```

## Production Build

For production, use Docker:

```powershell
docker-compose up -d
```

This builds and runs everything in containers.

