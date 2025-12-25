# Borg System Architecture

## System Overview

Borg is a distributed job execution system consisting of:
- **Mothership**: Central coordination server
- **Solder**: Worker agents that execute tasks
- **Web Dashboard**: React-based frontend for monitoring and management

## High-Level Architecture Diagram

```mermaid
graph TB
    subgraph "Client Layer"
        Web[Web Dashboard<br/>React/TypeScript]
        CLI[CLI Client<br/>HTTP REST]
    end
    
    subgraph "Mothership - Central Server"
        API[REST API Server<br/>Gin Framework<br/>Port 8080]
        WS[WebSocket Hub<br/>Real-time Updates]
        Queue[Queue System<br/>Task Distribution]
        Storage[Storage Service<br/>File Management]
        Auth[Authentication<br/>Service]
        
        API --> Queue
        API --> Storage
        API --> WS
        API --> Auth
    end
    
    subgraph "Database Layer"
        DB[(PostgreSQL<br/>Jobs, Tasks, Runners<br/>Files, Logs)]
    end
    
    subgraph "Solder Agents - Workers"
        Agent1[Solder Agent 1<br/>Downloader<br/>Executor<br/>Uploader]
        Agent2[Solder Agent 2<br/>Downloader<br/>Executor<br/>Uploader]
        Agent3[Solder Agent N<br/>Downloader<br/>Executor<br/>Uploader]
        
        Agent1 --> Heartbeat1[Heartbeat<br/>30s interval]
        Agent2 --> Heartbeat2[Heartbeat<br/>30s interval]
        Agent3 --> Heartbeat3[Heartbeat<br/>30s interval]
    end
    
    Web -->|HTTP/WS| API
    CLI -->|HTTP REST| API
    API --> DB
    Queue --> DB
    Storage -->|File Storage| FS[File System<br/>Volumes]
    
    Agent1 -->|HTTP| API
    Agent2 -->|HTTP| API
    Agent3 -->|HTTP| API
    
    Heartbeat1 -->|POST /heartbeat| API
    Heartbeat2 -->|POST /heartbeat| API
    Heartbeat3 -->|POST /heartbeat| API
    
    style API fill:#4A90E2,stroke:#2E5C8A,color:#fff
    style DB fill:#336791,stroke:#1A3F5C,color:#fff
    style Agent1 fill:#50C878,stroke:#2D8659,color:#fff
    style Agent2 fill:#50C878,stroke:#2D8659,color:#fff
    style Agent3 fill:#50C878,stroke:#2D8659,color:#fff
    style Web fill:#61DAFB,stroke:#2E86AB,color:#000
```

## Component Interaction Flow

### Job Submission Flow

```mermaid
sequenceDiagram
    participant User
    participant Web as Web Dashboard
    participant API as REST API
    participant Queue as Queue System
    participant DB as PostgreSQL
    participant Agent as Solder Agent
    
    User->>Web: Create Job
    Web->>API: POST /api/v1/jobs
    API->>Queue: EnqueueJob()
    Queue->>DB: Create Job & Task
    API-->>Web: Job Created
    Web-->>User: Success
    
    Agent->>API: GET /runners/:id/tasks/next (Poll)
    API->>Queue: GetNextTask()
    Queue->>DB: Query Pending Task
    DB-->>Queue: Return Task
    Queue->>DB: Update Task Status (running)
    Queue-->>API: Task Details
    API-->>Agent: Task Assignment
    
    Agent->>API: POST /files/:id/download
    API->>Storage: Get File
    Storage-->>API: File Data
    API-->>Agent: File Stream
    
    Agent->>Agent: Execute Task
    Agent->>API: POST /tasks/:id/status (updates)
    API->>DB: Update Task Status
    API->>WS: Broadcast Update
    WS-->>Web: Real-time Update
    
    Agent->>API: POST /artifacts/upload
    API->>Storage: Store Artifact
    API->>DB: Update Task (completed)
    API-->>Agent: Success
```

### Runner Registration & Heartbeat Flow

```mermaid
sequenceDiagram
    participant Agent as Solder Agent
    participant API as REST API
    participant DB as PostgreSQL
    participant WS as WebSocket Hub
    participant Web as Web Dashboard
    
    Agent->>Agent: Detect Resources (CPU, Memory, GPU, IPs)
    Agent->>API: POST /api/v1/runners/register
    Note over Agent,API: Name, Hostname, OS, Arch<br/>MaxConcurrentTasks, Labels<br/>Resource Info, Token
    API->>DB: Create/Update Runner
    DB-->>API: Runner ID
    API-->>Agent: Registration Success
    
    loop Every 30 seconds
        Agent->>API: POST /runners/:id/heartbeat
        Note over Agent,API: Status, ActiveTasks
        API->>DB: Update LastHeartbeat
        API->>WS: Broadcast Runner Status
        WS-->>Web: Real-time Status Update
        API-->>Agent: Heartbeat Response
    end
```

## Mothership Internal Architecture

```mermaid
graph LR
    subgraph "Mothership Service"
        Main[main.go<br/>Entry Point]
        
        subgraph "API Layer"
            Router[Gin Router<br/>/api/v1/*]
            Handlers[API Handlers<br/>Jobs, Runners, Tasks]
            Static[Static File Server<br/>Web App]
        end
        
        subgraph "Core Services"
            QueueService[Queue Service<br/>- EnqueueJob<br/>- GetNextTask<br/>- UpdateTaskStatus]
            StorageService[Storage Service<br/>- File Upload<br/>- File Download<br/>- Artifact Storage]
            WSHub[WebSocket Hub<br/>- Client Management<br/>- Broadcast Messages]
            AuthService[Auth Service<br/>- Token Validation]
        end
        
        subgraph "Data Models"
            JobModel[Job Model]
            TaskModel[Task Model]
            RunnerModel[Runner Model]
            FileModel[File Model]
            LogModel[TaskLog Model]
        end
        
        Main --> Router
        Router --> Handlers
        Router --> Static
        Router --> WSHub
        
        Handlers --> QueueService
        Handlers --> StorageService
        Handlers --> AuthService
        
        QueueService --> JobModel
        QueueService --> TaskModel
        QueueService --> RunnerModel
        
        StorageService --> FileModel
        Handlers --> LogModel
        
        JobModel --> DB[(PostgreSQL)]
        TaskModel --> DB
        RunnerModel --> DB
        FileModel --> DB
        LogModel --> DB
    end
    
    style Main fill:#E74C3C,stroke:#C0392B,color:#fff
    style QueueService fill:#3498DB,stroke:#2980B9,color:#fff
    style StorageService fill:#9B59B6,stroke:#8E44AD,color:#fff
    style WSHub fill:#F39C12,stroke:#E67E22,color:#fff
```

## Solder Agent Internal Architecture

```mermaid
graph TB
    subgraph "Solder Agent"
        Main[main.go<br/>Agent Entry Point]
        
        subgraph "Communication Layer"
            Client[HTTP Client<br/>REST API Client]
            HeartbeatService[Heartbeat Service<br/>30s Interval]
        end
        
        subgraph "Task Execution Pipeline"
            Poller[Job Poller<br/>StreamJobs<br/>5s Interval]
            Downloader[Downloader<br/>File Downloads]
            Executor[Executor<br/>Shell/Binary/Docker]
            Uploader[Uploader<br/>Artifact Uploads]
        end
        
        subgraph "Resource Detection"
            Resources[Resource Detector<br/>CPU, Memory, GPU, IPs]
        end
        
        subgraph "Work Directory"
            WorkDir[./work<br/>Task Workspace]
        end
        
        Main --> Client
        Main --> Resources
        Main --> HeartbeatService
        Main --> Poller
        
        Resources --> Client
        Client -->|Register| API[Mothership API]
        
        HeartbeatService -->|POST /heartbeat| API
        
        Poller -->|GET /tasks/next| API
        Poller -->|Task Assignment| Downloader
        
        Downloader -->|GET /files/:id/download| API
        Downloader --> WorkDir
        
        Downloader --> Executor
        Executor --> WorkDir
        
        Executor --> Uploader
        Uploader -->|POST /artifacts/upload| API
        
        Executor -->|POST /tasks/:id/status| API
        
        style Main fill:#2ECC71,stroke:#27AE60,color:#fff
        style Executor fill:#E67E22,stroke:#D35400,color:#fff
        style Client fill:#3498DB,stroke:#2980B9,color:#fff
        style HeartbeatService fill:#E74C3C,stroke:#C0392B,color:#fff
```

## Data Model Relationships

```mermaid
erDiagram
    JOB ||--o{ TASK : "has"
    JOB ||--o{ JOB_FILE : "requires"
    RUNNER ||--o{ TASK : "executes"
    TASK ||--o{ TASK_LOG : "generates"
    FILE ||--o{ JOB_FILE : "used_by"
    
    JOB {
        string id PK
        string name
        string type
        string status
        string command
        int priority
        datetime created_at
    }
    
    TASK {
        string id PK
        string job_id FK
        string runner_id FK
        string status
        int exit_code
        datetime started_at
        datetime completed_at
    }
    
    RUNNER {
        string id PK
        string device_id UK
        string name
        string status
        int max_concurrent_tasks
        int active_tasks
        datetime last_heartbeat
    }
    
    FILE {
        string id PK
        string filename
        int64 size
        string content_type
        string storage_path
    }
    
    JOB_FILE {
        int id PK
        string job_id FK
        string file_id FK
        string path
    }
    
    TASK_LOG {
        int id PK
        string task_id FK
        string level
        text message
        datetime timestamp
    }
```

## Technology Stack

### Mothership (Server)
- **Language**: Go
- **Web Framework**: Gin
- **Database**: PostgreSQL (via GORM)
- **WebSocket**: Gorilla WebSocket
- **File Storage**: Local filesystem (volumes)

### Solder (Agent)
- **Language**: Go
- **Communication**: HTTP REST API
- **Execution**: Shell, Binary, Docker containers
- **Resource Detection**: System calls

### Web Dashboard
- **Framework**: React + TypeScript
- **Build Tool**: Vite
- **Styling**: Tailwind CSS
- **Real-time**: WebSocket client

### Infrastructure
- **Containerization**: Docker & Docker Compose
- **Database**: PostgreSQL 14
- **Volume Management**: Docker volumes

## API Endpoints Overview

### Public API (Web Dashboard)
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

### Runner API (Solder Agents)
- `POST /api/v1/runners/register` - Register runner
- `POST /api/v1/runners/:id/heartbeat` - Send heartbeat
- `GET /api/v1/runners/:id/tasks/next` - Get next task
- `POST /api/v1/tasks/:id/status` - Update task status
- `GET /api/v1/files/:id/download` - Download file
- `POST /api/v1/artifacts/upload` - Upload artifact

### WebSocket
- `GET /ws` - WebSocket connection for real-time updates

## Deployment Architecture

```mermaid
graph TB
    subgraph "Docker Compose Environment"
        subgraph "Mothership Container"
            Mothership[Mothership Service<br/>Port 8080]
            MothershipVol[mothership_storage<br/>/app/storage]
        end
        
        subgraph "PostgreSQL Container"
            Postgres[PostgreSQL 14<br/>Port 5432]
            PostgresVol[postgres_data<br/>/var/lib/postgresql/data]
        end
        
        Mothership --> Postgres
        Mothership --> MothershipVol
        Postgres --> PostgresVol
    end
    
    subgraph "External Agents"
        Agent1[Solder Agent 1<br/>MOTHERSHIP_ADDR=http://host:8080]
        Agent2[Solder Agent 2<br/>MOTHERSHIP_ADDR=http://host:8080]
        AgentN[Solder Agent N<br/>MOTHERSHIP_ADDR=http://host:8080]
    end
    
    subgraph "Client Access"
        Browser[Web Browser<br/>http://localhost:8080]
        CLI2[CLI/API Clients]
    end
    
    Browser -->|HTTP/WS| Mothership
    CLI2 -->|HTTP REST| Mothership
    Agent1 -->|HTTP| Mothership
    Agent2 -->|HTTP| Mothership
    AgentN -->|HTTP| Mothership
    
    style Mothership fill:#4A90E2,stroke:#2E5C8A,color:#fff
    style Postgres fill:#336791,stroke:#1A3F5C,color:#fff
    style Agent1 fill:#50C878,stroke:#2D8659,color:#fff
    style Agent2 fill:#50C878,stroke:#2D8659,color:#fff
    style AgentN fill:#50C878,stroke:#2D8659,color:#fff
```

## Security Considerations

1. **Authentication**: Token-based authentication for runners
2. **Authorization**: Runner registration requires valid token
3. **Network**: Runners communicate via HTTP (can be upgraded to HTTPS)
4. **File Storage**: Files stored in isolated volumes
5. **Database**: Credentials via environment variables

## Scalability Notes

- **Horizontal Scaling**: Multiple Solder agents can be deployed
- **Load Distribution**: Queue system distributes tasks based on runner capacity
- **Concurrency**: Each runner can handle multiple concurrent tasks
- **Database**: PostgreSQL supports high concurrency
- **WebSocket**: Hub handles multiple concurrent connections

