# Build Frontend for Production
Write-Host "Building Frontend for Production..." -ForegroundColor Cyan
Write-Host ""

cd web

# Check if node_modules exists
if (-not (Test-Path "node_modules")) {
    Write-Host "Installing dependencies..." -ForegroundColor Yellow
    npm install
    if ($LASTEXITCODE -ne 0) {
        Write-Host "ERROR: Failed to install dependencies!" -ForegroundColor Red
        exit 1
    }
}

# Build frontend
Write-Host "Building frontend..." -ForegroundColor Yellow
npm run build

if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR: Frontend build failed!" -ForegroundColor Red
    exit 1
}

Write-Host ""
Write-Host "✓ Frontend built successfully!" -ForegroundColor Green
Write-Host "Build output: web/dist/" -ForegroundColor Green
Write-Host ""
Write-Host "You can now run the backend server and it will serve the frontend." -ForegroundColor Yellow

