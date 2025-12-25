package websocket

import (
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ScreenHub manages screen streaming sessions per runner
type ScreenHub struct {
	// Map of runnerID -> set of connected viewers
	viewers map[string]map[*ScreenClient]bool
	
	// Map of runnerID -> channel for frames from agent
	frameChannels map[string]chan []byte
	
	// Map of runnerID -> whether agent is currently streaming
	streaming map[string]bool
	
	// Callback to notify when streaming should start/stop
	onStreamingChange func(runnerID string, shouldStream bool)
	
	mu sync.RWMutex
}

// ScreenClient represents a WebSocket client viewing a screen stream
type ScreenClient struct {
	hub     *ScreenHub
	conn    *websocket.Conn
	runnerID string
	send    chan []byte
}

// NewScreenHub creates a new screen streaming hub
func NewScreenHub(onStreamingChange func(runnerID string, shouldStream bool)) *ScreenHub {
	return &ScreenHub{
		viewers:          make(map[string]map[*ScreenClient]bool),
		frameChannels:    make(map[string]chan []byte),
		streaming:        make(map[string]bool),
		onStreamingChange: onStreamingChange,
	}
}

// RegisterViewer registers a new viewer for a runner
func (h *ScreenHub) RegisterViewer(client *ScreenClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	if h.viewers[client.runnerID] == nil {
		h.viewers[client.runnerID] = make(map[*ScreenClient]bool)
		h.frameChannels[client.runnerID] = make(chan []byte, 10)
	}
	
	h.viewers[client.runnerID][client] = true
	
	// If this is the first viewer and not already streaming, start streaming
	if len(h.viewers[client.runnerID]) == 1 && !h.streaming[client.runnerID] {
		h.streaming[client.runnerID] = true
		if h.onStreamingChange != nil {
			go h.onStreamingChange(client.runnerID, true)
		}
	}
	
	log.Printf("Screen viewer registered for runner %s. Total viewers: %d", client.runnerID, len(h.viewers[client.runnerID]))
}

// UnregisterViewer unregisters a viewer
func (h *ScreenHub) UnregisterViewer(client *ScreenClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	if viewers, ok := h.viewers[client.runnerID]; ok {
		if _, exists := viewers[client]; exists {
			delete(viewers, client)
			close(client.send)
			
			// If no viewers remain, stop streaming
			if len(viewers) == 0 {
				delete(h.viewers, client.runnerID)
				close(h.frameChannels[client.runnerID])
				delete(h.frameChannels, client.runnerID)
				h.streaming[client.runnerID] = false
				if h.onStreamingChange != nil {
					go h.onStreamingChange(client.runnerID, false)
				}
				log.Printf("No viewers remaining for runner %s. Streaming stopped.", client.runnerID)
			} else {
				log.Printf("Screen viewer unregistered for runner %s. Remaining viewers: %d", client.runnerID, len(viewers))
			}
		}
	}
}

// BroadcastFrame sends a frame to all viewers of a runner
func (h *ScreenHub) BroadcastFrame(runnerID string, frameData []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	if frameChan, ok := h.frameChannels[runnerID]; ok {
		select {
		case frameChan <- frameData:
		default:
			// Channel full, drop frame
			log.Printf("Frame channel full for runner %s, dropping frame", runnerID)
		}
	}
}

// GetFrameChannel returns the frame channel for a runner
func (h *ScreenHub) GetFrameChannel(runnerID string) <-chan []byte {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	if ch, ok := h.frameChannels[runnerID]; ok {
		return ch
	}
	return nil
}

// IsStreaming returns whether a runner is currently streaming
func (h *ScreenHub) IsStreaming(runnerID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	return h.streaming[runnerID]
}

// ViewerCount returns the number of viewers for a runner
func (h *ScreenHub) ViewerCount(runnerID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	
	if viewers, ok := h.viewers[runnerID]; ok {
		return len(viewers)
	}
	return 0
}

// ScreenFrameMessage is deprecated - frames are now sent as binary messages
// Kept for reference but no longer used

// NewScreenClient creates a new screen viewing client
func NewScreenClient(hub *ScreenHub, conn *websocket.Conn, runnerID string) *ScreenClient {
	return &ScreenClient{
		hub:     hub,
		conn:    conn,
		runnerID: runnerID,
		send:    make(chan []byte, 256),
	}
}

const (
	// Time allowed to write a message to the peer
	screenWriteWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	screenPongWait = 60 * time.Second
	// Send pings to peer with this period (must be less than pongWait)
	screenPingPeriod = (screenPongWait * 9) / 10
	// Maximum message size allowed from peer
	screenMaxMessageSize = 10 * 1024 * 1024 // 10MB for frames
)

// ReadPump pumps messages from the websocket connection
func (c *ScreenClient) ReadPump() {
	defer func() {
		c.hub.UnregisterViewer(c)
		c.conn.Close()
	}()
	
	c.conn.SetReadDeadline(time.Now().Add(screenPongWait))
	c.conn.SetReadLimit(screenMaxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(screenPongWait))
		return nil
	})
	
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Screen WebSocket error: %v", err)
			}
			break
		}
	}
}

// WritePump pumps messages to the websocket connection
func (c *ScreenClient) WritePump() {
	ticker := time.NewTicker(screenPingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	
	// Start receiving frames from the hub
	frameChan := c.hub.GetFrameChannel(c.runnerID)
	if frameChan == nil {
		log.Printf("No frame channel available for runner %s", c.runnerID)
		return
	}
	
	for {
		select {
		case frameData, ok := <-frameChan:
			c.conn.SetWriteDeadline(time.Now().Add(screenWriteWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			// Send binary frame directly (no base64 encoding, no JSON)
			// frameData is binary JPEG data
			err := c.conn.WriteMessage(websocket.BinaryMessage, frameData)
			if err != nil {
				log.Printf("Error writing binary frame: %v", err)
				return
			}
			
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(screenWriteWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			
			if err := w.Close(); err != nil {
				return
			}
			
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(screenWriteWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

