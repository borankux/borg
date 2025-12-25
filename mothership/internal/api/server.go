package api

import (
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
}

// NewServer creates a new API server
func NewServer(db *gorm.DB, q *queue.Queue, hub *websocket.Hub, screenHub *websocket.ScreenHub, storage *storage.Storage) *Server {
	handler := NewHandler(db, q, storage, screenHub)
	
	router := gin.Default()
	
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
	
	// Screen streaming WebSocket endpoint
	router.GET("/ws/screen/:runnerID", websocket.HandleScreenWebSocket(screenHub))
	
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
			protected.DELETE("/runners/:id", handler.DeleteRunner)
			
			// Logs
			protected.GET("/tasks/:id/logs", handler.GetTaskLogs)
			
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
		
		// Screen streaming endpoints (unprotected - for agents)
		api.POST("/runners/:id/screen/frame", handler.UploadScreenFrame)
		api.GET("/runners/:id/screen/status", handler.GetScreenStreamStatus)
		
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

// GetRouter returns the router (for WebSocket setup)
func (s *Server) GetRouter() *gin.Engine {
	return s.router
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

