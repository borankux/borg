package websocket

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var screenUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// HandleScreenWebSocket handles WebSocket connections for screen streaming
func HandleScreenWebSocket(hub *ScreenHub) gin.HandlerFunc {
	return func(c *gin.Context) {
		runnerID := c.Param("runnerID")
		if runnerID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "runnerID is required"})
			return
		}
		
		conn, err := screenUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		
		client := NewScreenClient(hub, conn, runnerID)
		hub.RegisterViewer(client)
		
		// Allow collection of memory referenced by the caller by doing all work in new goroutines
		go client.WritePump()
		go client.ReadPump()
	}
}

