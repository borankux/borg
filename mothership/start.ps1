# Mothership Startup Script
Write-Host "Checking prerequisites..."

# Check if protoc is installed
$protocInstalled = Get-Command protoc -ErrorAction SilentlyContinue
if (-not $protocInstalled) {
    Write-Host "ERROR: protoc is not installed!" -ForegroundColor Red
    Write-Host "Please install protoc from: https://github.com/protocolbuffers/protobuf/releases" -ForegroundColor Yellow
    Write-Host "Or install via chocolatey: choco install protoc" -ForegroundColor Yellow
    exit 1
}

Write-Host "✓ protoc found" -ForegroundColor Green

# Check if proto files are generated
if (-not (Test-Path "pkg\proto\borg.pb.go")) {
    Write-Host "Generating proto files..."
    & protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pkg/proto/borg.proto
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to generate proto files!" -ForegroundColor Red
        exit 1
    }
    Write-Host "✓ Proto files generated" -ForegroundColor Green
} else {
    Write-Host "✓ Proto files already exist" -ForegroundColor Green
}

# Download dependencies
Write-Host "Downloading Go dependencies..."
go mod download
if ($LASTEXITCODE -ne 0) {
    Write-Host "WARNING: Some dependencies may not have downloaded. Continuing anyway..." -ForegroundColor Yellow
}

# Start the server
Write-Host "Starting mothership server..." -ForegroundColor Cyan
Write-Host ""
go run cmd/server/main.go

