# Development Script - Run Backend and Frontend
# Make sure PostgreSQL is running in Docker first: docker-compose up -d postgres

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Borg Development Environment" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Check if postgres is running
$postgresRunning = docker ps --filter "name=postgres" --format "{{.Names}}" | Select-String "postgres"
if (-not $postgresRunning) {
    Write-Host "ERROR: PostgreSQL container is not running!" -ForegroundColor Red
    Write-Host "Please start it with: docker-compose up -d postgres" -ForegroundColor Yellow
    exit 1
}

Write-Host "✓ PostgreSQL is running" -ForegroundColor Green
Write-Host ""

# Start backend in a new window
Write-Host "Starting Backend Server..." -ForegroundColor Cyan
Start-Process pwsh -ArgumentList "-NoExit", "-File", "mothership/run-backend.ps1" -WindowStyle Normal

# Wait a bit for backend to start
Start-Sleep -Seconds 3

# Start frontend in a new window
Write-Host "Starting Frontend Server..." -ForegroundColor Cyan
Start-Process pwsh -ArgumentList "-NoExit", "-File", "mothership/web/run-frontend.ps1" -WindowStyle Normal

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Development servers started!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Backend:  http://localhost:8080" -ForegroundColor Yellow
Write-Host "Frontend: http://localhost:5173" -ForegroundColor Yellow
Write-Host ""
Write-Host "Press Ctrl+C to stop this script (servers will continue running)" -ForegroundColor Gray
Write-Host "Close the server windows to stop them" -ForegroundColor Gray

# Keep script running
try {
    while ($true) {
        Start-Sleep -Seconds 1
    }
} finally {
    Write-Host "`nShutting down..." -ForegroundColor Yellow
}

