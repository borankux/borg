package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

// ServeStaticFiles serves the web frontend static files
func ServeStaticFiles(router *gin.Engine) {
	// Try multiple possible paths for dist directory
	possiblePaths := []string{
		"./web/dist",                    // Relative to working directory
		"../web/dist",                   // If running from cmd/server
		"./mothership/web/dist",         // If running from project root
		"web/dist",                      // Alternative relative path
	}
	
	var distPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			absPath, err := filepath.Abs(path)
			if err == nil {
				distPath = absPath
				break
			}
		}
	}
	
	if distPath != "" {
		// Serve static files
		router.StaticFile("/", filepath.Join(distPath, "index.html"))
		router.Static("/assets", filepath.Join(distPath, "assets"))
		
		// Serve other static files (favicon, etc.)
		router.StaticFS("/static", http.Dir(distPath))
		
		// Catch-all for SPA routing - serve index.html for all non-API routes
		router.NoRoute(func(c *gin.Context) {
			// Don't serve index.html for API routes
			if !strings.HasPrefix(c.Request.URL.Path, "/api") && 
			   !strings.HasPrefix(c.Request.URL.Path, "/ws") {
				c.File(filepath.Join(distPath, "index.html"))
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			}
		})
	} else {
		// Development mode - serve a helpful message
		router.NoRoute(func(c *gin.Context) {
			if strings.HasPrefix(c.Request.URL.Path, "/api") || 
			   strings.HasPrefix(c.Request.URL.Path, "/ws") {
				c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
			} else {
				c.JSON(http.StatusNotFound, gin.H{
					"message": "Web frontend not built. Please run 'npm run build' in the web directory.",
					"hint":    "Run: cd web && npm run build",
				})
			}
		})
	}
}

