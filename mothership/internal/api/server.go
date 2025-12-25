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
		// Dashboard
		api.GET("/stats", handler.GetDashboardStats)
		
		// Jobs
		api.GET("/jobs", handler.ListJobs)
		api.POST("/jobs", handler.CreateJob)
		api.GET("/jobs/:id", handler.GetJob)
		api.POST("/jobs/:id/pause", handler.PauseJob)
		api.POST("/jobs/:id/resume", handler.ResumeJob)
		api.POST("/jobs/:id/cancel", handler.CancelJob)
		
		// Runners
		api.GET("/runners", handler.ListRunners)
		api.GET("/runners/:id", handler.GetRunner)
		api.PATCH("/runners/:id/rename", handler.RenameRunner)
		api.DELETE("/runners/:id", handler.DeleteRunner)
		
		// Logs
		api.GET("/tasks/:id/logs", handler.GetTaskLogs)
		
		// Runner API endpoints
		api.POST("/runners/register", handler.RegisterRunner)
		api.POST("/runners/:id/heartbeat", handler.Heartbeat)
		api.GET("/runners/:id/tasks/next", handler.GetNextTask)
		api.POST("/tasks/:id/status", handler.UpdateTaskStatus)
		api.GET("/files/:id/download", handler.DownloadFile)
		api.POST("/artifacts/upload", handler.UploadArtifact)
		
		// Screen streaming endpoints
		api.POST("/runners/:id/screen/frame", handler.UploadScreenFrame)
		api.GET("/runners/:id/screen/status", handler.GetScreenStreamStatus)
		
		// Screenshot endpoints (deprecated - kept for backward compatibility)
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

