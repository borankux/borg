package websocket

import (
	"encoding/base64"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var screenUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins in development
	},
}

const (
	// Time allowed to read the next message from agent
	agentReadWait = 60 * time.Second
	// Maximum message size from agent (10MB for frames)
	agentMaxMessageSize = 10 * 1024 * 1024
	// Time allowed to write a message to agent
	agentWriteWait = 10 * time.Second
	// Send pings to agent with this period
	agentPingPeriod = (agentReadWait * 9) / 10
)

// HandleScreenWebSocket handles WebSocket connections for screen streaming (viewers)
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

// HandleAgentScreenWebSocket handles WebSocket connections from agents for sending frames
func HandleAgentScreenWebSocket(hub *ScreenHub) gin.HandlerFunc {
	return func(c *gin.Context) {
		runnerID := c.Param("runnerID")
		if runnerID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "runnerID is required"})
			return
		}
		
		conn, err := screenUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("Failed to upgrade agent WebSocket connection: %v", err)
			return
		}
		
		log.Printf("Agent WebSocket connected for runner %s", runnerID)
		
		// Set up connection parameters
		conn.SetReadDeadline(time.Now().Add(agentReadWait))
		conn.SetReadLimit(agentMaxMessageSize)
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(agentReadWait))
			return nil
		})
		
		// Start ping ticker
		ticker := time.NewTicker(agentPingPeriod)
		defer func() {
			ticker.Stop()
			conn.Close()
			log.Printf("Agent WebSocket disconnected for runner %s", runnerID)
		}()
		
		// Ping goroutine
		go func() {
			for {
				select {
				case <-ticker.C:
					conn.SetWriteDeadline(time.Now().Add(agentWriteWait))
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						return
					}
				}
			}
		}()
		
		// Read frames from agent (binary messages)
		for {
			messageType, frameData, err := conn.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("Agent WebSocket error for runner %s: %v", runnerID, err)
				}
				break
			}
			
			if messageType == websocket.BinaryMessage {
				// Convert binary JPEG to base64 data URL format for viewers
				frameDataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(frameData)
				hub.BroadcastFrame(runnerID, []byte(frameDataURL))
			} else if messageType == websocket.TextMessage {
				// Handle text messages (ping/pong, etc.)
				log.Printf("Received text message from agent %s: %s", runnerID, string(frameData))
			}
		}
	}
}

