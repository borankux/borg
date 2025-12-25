package websocket

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

// HandleWebSocket handles WebSocket connections
func HandleWebSocket(hub *Hub) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		
		client := NewClient(hub, conn)
		client.hub.register <- client
		
		// Allow collection of memory referenced by the caller by doing all work in new goroutines
		go client.WritePump()
		go client.ReadPump()
	}
}

