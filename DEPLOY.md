# Deployment Guide for Baota Linux Panel (Without Docker)

This guide explains how to deploy the Borg system on Baota Linux panel without Docker.

## Prerequisites

1. **Baota Linux Panel** installed
2. **Go** installed (1.21+)
3. **Node.js & npm** installed
4. **PostgreSQL** database (can be in Docker or native)

## Step 1: Build Frontend

```bash
cd mothership/web
npm install
npm run build
```

Or use the script:
```bash
cd mothership
./build-frontend.ps1  # On Windows
# or
bash build-frontend.sh  # On Linux (if created)
```

This creates `mothership/web/dist/` with the built frontend files.

## Step 2: Configure Backend

The backend will automatically serve the frontend from `web/dist/` directory.

### Environment Variables

Create a `.env` file in the `mothership` directory:

```env
DATABASE_URL=host=localhost user=postgres password=your_password dbname=borg port=5432 sslmode=disable
STORAGE_PATH=./storage
HTTP_PORT=8080
```

Or set them in Baota panel's environment variables.

## Step 3: Build Backend Binary

```bash
cd mothership
go build -o server cmd/server/main.go
```

This creates a `server` executable.

## Step 4: Setup in Baota Panel

### Option A: Run as System Service

1. **Create systemd service file** (`/etc/systemd/system/borg-mothership.service`):

```ini
[Unit]
Description=Borg Mothership Server
After=network.target postgresql.service

[Service]
Type=simple
User=www
WorkingDirectory=/www/wwwroot/your-domain/mothership
ExecStart=/www/wwwroot/your-domain/mothership/server
Restart=always
RestartSec=10
Environment="DATABASE_URL=host=localhost user=postgres password=your_password dbname=borg port=5432 sslmode=disable"
Environment="STORAGE_PATH=./storage"
Environment="HTTP_PORT=8080"

[Install]
WantedBy=multi-user.target
```

2. **Enable and start service**:
```bash
sudo systemctl enable borg-mothership
sudo systemctl start borg-mothership
sudo systemctl status borg-mothership
```

### Option B: Use Baota's PM2 Manager

1. In Baota panel, go to **PM2 Manager**
2. Add new project:
   - **Project name**: borg-mothership
   - **Project path**: `/www/wwwroot/your-domain/mothership`
   - **Startup file**: `server`
   - **Project port**: `8080`
3. Add environment variables:
   ```
   DATABASE_URL=host=localhost user=postgres password=your_password dbname=borg port=5432 sslmode=disable
   STORAGE_PATH=./storage
   HTTP_PORT=8080
   ```

## Step 5: Configure Reverse Proxy in Baota

1. Go to **Website** → Your domain → **Settings**
2. Click **Reverse Proxy**
3. Add reverse proxy:
   - **Proxy name**: borg-api
   - **Target URL**: `http://127.0.0.1:8080`
   - **Send domain**: Enable
   - **Proxy directory**: `/`

Or configure Nginx manually:

```nginx
server {
    listen 80;
    server_name your-domain.com;
    
    # Serve frontend static files (optional - backend also serves them)
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
    
    # WebSocket support
    location /ws {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

## Step 6: Create Storage Directory

```bash
cd mothership
mkdir -p storage/{files,artifacts,tmp,screenshots}
chmod -R 755 storage
```

## Step 7: Database Setup

If using PostgreSQL in Docker:
```bash
docker run -d \
  --name borg-postgres \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=your_password \
  -e POSTGRES_DB=borg \
  -p 5432:5432 \
  postgres:14
```

Or use Baota's PostgreSQL manager.

## Step 8: Verify Deployment

1. **Check backend is running**:
   ```bash
   curl http://localhost:8080/api/v1/stats
   ```

2. **Check frontend is served**:
   ```bash
   curl http://localhost:8080/
   ```

3. **Access web interface**:
   Open `http://your-domain.com` in browser

## Troubleshooting

### Frontend not showing

1. Check if `web/dist/` exists and has files:
   ```bash
   ls -la mothership/web/dist/
   ```

2. Rebuild frontend:
   ```bash
   cd mothership/web
   npm run build
   ```

3. Check backend logs for path resolution

### Backend can't find dist folder

The backend tries multiple paths:
- `./web/dist` (relative to working directory)
- `../web/dist` (if running from cmd/server)
- `./mothership/web/dist` (if running from project root)

Make sure you run the backend from the `mothership` directory or adjust the path.

### Database connection issues

1. Verify PostgreSQL is running:
   ```bash
   sudo systemctl status postgresql
   ```

2. Test connection:
   ```bash
   psql -h localhost -U postgres -d borg
   ```

3. Check firewall:
   ```bash
   sudo ufw allow 5432
   ```

## Updating Frontend

After making frontend changes:

```bash
cd mothership/web
npm run build
# Restart backend service
sudo systemctl restart borg-mothership
```

## File Structure

```
mothership/
├── server              # Compiled binary
├── web/
│   └── dist/          # Built frontend (must exist)
│       ├── index.html
│       └── assets/
├── storage/           # File storage
│   ├── files/
│   ├── artifacts/
│   ├── tmp/
│   └── screenshots/
└── .env              # Environment variables
```

## Security Notes

1. **Firewall**: Only expose port 80/443 (via Nginx), not 8080 directly
2. **Database**: Use strong passwords and restrict access
3. **Storage**: Set proper file permissions
4. **SSL**: Use Baota's SSL certificate manager for HTTPS

