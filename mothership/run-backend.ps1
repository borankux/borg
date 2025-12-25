# Run Mothership Backend (Go server)
Write-Host "Starting Mothership Backend..." -ForegroundColor Cyan
Write-Host ""

# Set environment variables
$env:DATABASE_URL = "host=localhost user=postgres password=postgres dbname=borg port=5432 sslmode=disable"
$env:STORAGE_PATH = "./storage"
$env:HTTP_PORT = "8080"

# Create storage directory if it doesn't exist
if (-not (Test-Path "./storage")) {
    New-Item -ItemType Directory -Path "./storage" | Out-Null
    Write-Host "Created storage directory" -ForegroundColor Green
}

# Run the Go server
Write-Host "Connecting to database at localhost:5432" -ForegroundColor Yellow
Write-Host "API will be available at http://0.0.0.0:8080 (accessible from all network interfaces)" -ForegroundColor Yellow
Write-Host ""

go run cmd/server/main.go

