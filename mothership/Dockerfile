# Backend build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Set GOPROXY to use 七牛云 (Qiniu Cloud) proxy
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.org

# Copy go mod files first
COPY go.mod ./
COPY go.sum* ./

# Download dependencies
RUN go mod download

# Copy all source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/server/main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/server .

# Copy frontend build (pre-built locally)
COPY web/dist ./web/dist

# Create storage directory
RUN mkdir -p /app/storage

EXPOSE 8080

CMD ["./server"]

