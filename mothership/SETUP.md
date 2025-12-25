# Distributed Task Execution System Setup Instructions

## Prerequisites

1. **Install Protocol Buffers Compiler (protoc)**
   - Download from: https://github.com/protocolbuffers/protobuf/releases
   - Extract and add to PATH
   - Or use package manager:
     - Windows (chocolatey): `choco install protoc`
     - Mac: `brew install protobuf`
     - Linux: `apt-get install protobuf-compiler` or `yum install protobuf-compiler`

2. **Generate Proto Files**
   ```bash
   cd mothership
   make proto
   ```
   Or manually:
   ```bash
   protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative pkg/proto/borg.proto
   ```

3. **Install Go Dependencies**
   ```bash
   go mod download
   go mod tidy
   ```

4. **Start PostgreSQL** (if not already running)
   ```bash
   docker-compose up -d postgres
   ```

5. **Run the Server**
   ```bash
   go run cmd/server/main.go
   ```

## Environment Variables

Create a `.env` file or set:
- `DATABASE_URL`: PostgreSQL connection string (default: `host=localhost user=postgres password=postgres dbname=borg port=5432 sslmode=disable`)
- `STORAGE_PATH`: Path for file storage (default: `./storage`)
- `HTTP_PORT`: HTTP server port (default: `8080`)
- `GRPC_PORT`: gRPC server port (default: `50051`)

