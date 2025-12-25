package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// ServeStaticFiles serves the web frontend static files
func ServeStaticFiles(router *gin.Engine) {
	// Try to serve from dist directory (relative to working directory)
	distPath := "./web/dist"
	
	if _, err := os.Stat(distPath); err == nil {
		// Serve static files
		router.StaticFile("/", filepath.Join(distPath, "index.html"))
		router.Static("/assets", filepath.Join(distPath, "assets"))
		
		// Catch-all for SPA routing
		router.NoRoute(func(c *gin.Context) {
			c.File(filepath.Join(distPath, "index.html"))
		})
	} else {
		// Development mode - serve a simple message
		router.NoRoute(func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{
				"message": "Web frontend not built. The Docker image needs to be rebuilt to include the frontend.",
			})
		})
	}
}

