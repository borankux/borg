package api

import (
	"fmt"
	"log"
	"net/http"

	"borg/mothership/internal/queue"
	"borg/mothership/internal/storage"
	"borg/mothership/internal/websocket"
	
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Server wraps the REST API server
type Server struct {
	handler   *Handler
	router    *gin.Engine
	hub       *websocket.Hub
	screenHub *websocket.ScreenHub
	agentHub  *websocket.AgentHub
}

// NewServer creates a new API server
func NewServer(db *gorm.DB, q *queue.Queue, hub *websocket.Hub, screenHub *websocket.ScreenHub, agentHub *websocket.AgentHub, storage *storage.Storage) *Server {
	handler := NewHandler(db, q, storage, screenHub, agentHub)
	
	// Use gin.New() instead of gin.Default() to avoid default logging
	// We'll add a custom logger that skips verbose endpoints
	router := gin.New()
	
	// Custom logger that skips logging for screen frame endpoint
	router.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Skip logging for screen frame endpoint (too verbose - frames come frequently)
		if len(param.Path) > 20 && param.Path[len(param.Path)-13:] == "/screen/frame" {
			return ""
		}
		// Default log format for other endpoints
		return fmt.Sprintf("[%s] %s %s %d %s %s \"%s\" %s\n",
			param.TimeStamp.Format("2006/01/02 - 15:04:05"),
			param.ClientIP,
			param.Method,
			param.StatusCode,
			param.Latency,
			param.Path,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
	}))
	router.Use(gin.Recovery())
	
	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	})
	
	// WebSocket endpoint
	router.GET("/ws", websocket.HandleWebSocket(hub))
	
	// Screen streaming WebSocket endpoint (for viewers)
	router.GET("/ws/screen/:runnerID", websocket.HandleScreenWebSocket(screenHub))
	
	// Screen streaming WebSocket endpoint (for agents to send frames)
	router.GET("/ws/screen/agent/:runnerID", websocket.HandleAgentScreenWebSocket(screenHub))
	
	// Agent WebSocket endpoint (for real-time communication)
	router.GET("/ws/agent/:runnerID", websocket.HandleAgentWebSocket(agentHub, db))
	
	// Download endpoint (before API routes)
	router.GET("/api/v1/download/solder.exe", handler.DownloadSolder)
	
	// API routes
	api := router.Group("/api/v1")
	{
		// Public auth endpoints (no authentication required)
		api.POST("/auth/login", handler.Login)
		
		// Protected dashboard endpoints (require authentication)
		protected := api.Group("")
		protected.Use(AuthMiddleware())
		{
			// Dashboard
			protected.GET("/stats", handler.GetDashboardStats)
			
			// Jobs
			protected.GET("/jobs", handler.ListJobs)
			protected.POST("/jobs", handler.CreateJob)
			protected.GET("/jobs/:id", handler.GetJob)
			protected.POST("/jobs/:id/pause", handler.PauseJob)
			protected.POST("/jobs/:id/resume", handler.ResumeJob)
			protected.POST("/jobs/:id/cancel", handler.CancelJob)
			
			// Runners (dashboard endpoints - protected)
			protected.GET("/runners", handler.ListRunners)
			protected.GET("/runners/:id", handler.GetRunner)
			protected.PATCH("/runners/:id/rename", handler.RenameRunner)
			protected.PATCH("/runners/:id/screen-settings", handler.UpdateScreenSettings)
			protected.DELETE("/runners/:id", handler.DeleteRunner)
			
			// Logs
			protected.GET("/tasks/:id/logs", handler.GetTaskLogs)
			
			// Executor binaries
			protected.POST("/executor-binaries/upload", handler.UploadExecutorBinary)
			protected.GET("/executor-binaries", handler.ListExecutorBinaries)
			protected.GET("/executor-binaries/:id", handler.GetExecutorBinary)
			protected.DELETE("/executor-binaries/:id", handler.DeleteExecutorBinary)

			// Job processor scripts and datasets
			protected.POST("/jobs/:id/processor-script/upload", handler.UploadProcessorScript)
			protected.POST("/jobs/:id/dataset/upload", handler.UploadCSVDataset)
			protected.GET("/jobs/:id/results", handler.ListJobResults)

			// Current user endpoint
			protected.GET("/auth/me", handler.GetCurrentUser)
		}
		
		// Runner API endpoints (unprotected - for agents)
		api.POST("/runners/register", handler.RegisterRunner)
		api.POST("/runners/:id/heartbeat", handler.Heartbeat)
		api.GET("/runners/:id/tasks/next", handler.GetNextTask)
		api.POST("/tasks/:id/status", handler.UpdateTaskStatus)
		api.GET("/files/:id/download", handler.DownloadFile)
		api.POST("/artifacts/upload", handler.UploadArtifact)

		// Job results upload (for solder agents)
		api.POST("/jobs/:id/results/upload", handler.UploadJobResult)

		// Screen streaming endpoints (unprotected - for agents)
		api.POST("/runners/:id/screen/frame", handler.UploadScreenFrame)
		api.GET("/runners/:id/screen/status", handler.GetScreenStreamStatus)
		
		// Screen information endpoint (protected - for dashboard)
		protected.GET("/runners/:id/screens", handler.GetAvailableScreens)
		
		// Screenshot endpoints (deprecated - kept for backward compatibility, unprotected)
		api.POST("/runners/:id/screenshots", handler.UploadScreenshot)
		api.GET("/runners/:id/screenshots", handler.GetScreenshots)
		api.GET("/runners/:id/screenshots/:filename", handler.GetScreenshot)
	}
	
	// Serve static files (web app) - must be last
	ServeStaticFiles(router)
	
	return &Server{
		handler:   handler,
		router:    router,
		hub:       hub,
		screenHub: screenHub,
		agentHub:  agentHub,
	}
}

// GetHub returns the WebSocket hub
func (s *Server) GetHub() *websocket.Hub {
	return s.hub
}

// GetScreenHub returns the screen streaming hub
func (s *Server) GetScreenHub() *websocket.ScreenHub {
	return s.screenHub
}

// GetAgentHub returns the agent WebSocket hub
func (s *Server) GetAgentHub() *websocket.AgentHub {
	return s.agentHub
}

// SetupAgentMessageHandler sets up the callback for handling agent WebSocket messages
func (s *Server) SetupAgentMessageHandler() {
	s.agentHub.SetMessageHandler(func(runnerID string, messageType string, data interface{}) {
		// Handle incoming WebSocket messages from agents
		switch messageType {
		case "heartbeat":
			s.handler.handleWebSocketHeartbeat(runnerID, data)
		case "task_status":
			s.handler.handleWebSocketTaskStatus(runnerID, data)
		default:
			log.Printf("Unknown message type from runner %s: %s", runnerID, messageType)
		}
	})
}

// GetRouter returns the router (for WebSocket setup)
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

