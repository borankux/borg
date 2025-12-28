package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// AgentWebSocketClient manages WebSocket connection for agent
type AgentWebSocketClient struct {
	baseURL    string
	runnerID   string
	conn       *websocket.Conn
	connMu     sync.Mutex
	dialer     *websocket.Dialer
	connected  bool
	reconnect  bool
	stopChan   chan struct{}
	messageChan chan *AgentMessage
	sendChan   chan []byte // Channel for sending messages (writePump reads from this)
}

// AgentMessage represents a WebSocket message
type AgentMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

const (
	// Time allowed to write a message to the peer
	agentWriteWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer
	agentPongWait = 60 * time.Second
	// Send pings to peer with this period (must be less than pongWait)
	agentPingPeriod = (agentPongWait * 9) / 10
	// Maximum message size allowed from peer
	agentMaxMessageSize = 512 * 1024 // 512KB
	// Reconnect delay
	reconnectDelay = 5 * time.Second
)

// NewAgentWebSocketClient creates a new WebSocket client
func NewAgentWebSocketClient(baseURL, runnerID string) *AgentWebSocketClient {
	return &AgentWebSocketClient{
		baseURL:     baseURL,
		runnerID:    runnerID,
		dialer:      &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
		reconnect:   true,
		stopChan:    make(chan struct{}),
		messageChan: make(chan *AgentMessage, 256),
		sendChan:    make(chan []byte, 256), // Buffer up to 256 messages - all writes go through this
	}
}

// Connect establishes WebSocket connection
func (c *AgentWebSocketClient) Connect(ctx context.Context) error {
	c.connMu.Lock()
	
	if c.conn != nil {
		c.connMu.Unlock()
		return nil // Already connected
	}

	// Recreate sendChan if needed (it may have been closed on disconnect)
	c.sendChan = make(chan []byte, 256)
	c.stopChan = make(chan struct{})
	
	c.connMu.Unlock()

	// Convert HTTP URL to WebSocket URL
	wsURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}

	if wsURL.Scheme == "https" {
		wsURL.Scheme = "wss"
	} else {
		wsURL.Scheme = "ws"
	}
	wsURL.Path = fmt.Sprintf("/ws/agent/%s", c.runnerID)

	conn, _, err := c.dialer.DialContext(ctx, wsURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}

	c.conn = conn
	c.connected = true

	// Set up connection parameters
	conn.SetReadDeadline(time.Now().Add(agentPongWait))
	conn.SetReadLimit(agentMaxMessageSize)
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(agentPongWait))
		return nil
	})

	log.Printf("WebSocket connected to mothership: %s", wsURL.String())

	// Start read and write pumps
	go c.readPump()
	go c.writePump()

	return nil
}

// Disconnect closes the WebSocket connection
func (c *AgentWebSocketClient) Disconnect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	c.reconnect = false
	c.connected = false
	
	// Close stop channel to signal pumps to stop (writePump will exit)
	if c.stopChan != nil {
		select {
		case <-c.stopChan:
			// Already closed, create new one for next connection
		default:
			close(c.stopChan)
		}
	}
	
	// Close send channel to signal writePump to stop
	if c.sendChan != nil {
		close(c.sendChan)
	}

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}
	return nil
}

// SendMessage sends a message to the mothership
// This method is safe to call from multiple goroutines - all writes go through sendChan
func (c *AgentWebSocketClient) SendMessage(messageType string, data interface{}) error {
	c.connMu.Lock()
	conn := c.conn
	c.connMu.Unlock()

	if conn == nil {
		return fmt.Errorf("not connected")
	}

	message := AgentMessage{
		Type: messageType,
	}

	if data != nil {
		dataBytes, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal data: %w", err)
		}
		message.Data = dataBytes
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Send message through channel - writePump will handle the actual write
	select {
	case c.sendChan <- messageBytes:
		return nil
	default:
		return fmt.Errorf("send channel full, message dropped")
	}
}

// ReceiveMessages returns a channel for receiving messages
func (c *AgentWebSocketClient) ReceiveMessages() <-chan *AgentMessage {
	return c.messageChan
}

// IsConnected returns whether the client is connected
func (c *AgentWebSocketClient) IsConnected() bool {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	return c.connected && c.conn != nil
}

// readPump reads messages from the WebSocket connection
func (c *AgentWebSocketClient) readPump() {
	defer func() {
		c.connMu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.connected = false
		// Don't close channels here - let Disconnect handle that
		c.connMu.Unlock()

		// Attempt to reconnect if enabled
		if c.reconnect {
			go c.reconnectLoop()
		}
	}()

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket read error: %v", err)
			}
			break
		}

		var message AgentMessage
		if err := json.Unmarshal(messageBytes, &message); err != nil {
			log.Printf("Failed to unmarshal WebSocket message: %v", err)
			continue
		}

		// Handle pong
		if message.Type == "pong" {
			continue
		}

		// Send message to channel
		select {
		case c.messageChan <- &message:
		default:
			log.Printf("Message channel full, dropping message")
		}
	}
}

// writePump writes messages to the WebSocket connection
// This is the only goroutine that writes to the connection, preventing concurrent writes
func (c *AgentWebSocketClient) writePump() {
	ticker := time.NewTicker(agentPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopChan:
			return
		case message, ok := <-c.sendChan:
			c.connMu.Lock()
			conn := c.conn
			c.connMu.Unlock()

			if conn == nil {
				return
			}

			conn.SetWriteDeadline(time.Now().Add(agentWriteWait))
			if !ok {
				// Channel closed, send close message
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Write the message - use NextWriter to allow batching if needed
			w, err := conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Try to batch any pending messages (drain the channel)
			// This helps reduce the number of websocket frames when many messages are queued
			batchLoop:
			for {
				select {
				case queuedMsg, ok := <-c.sendChan:
					if !ok {
						// Channel closed while draining
						w.Close()
						return
					}
					w.Write([]byte{'\n'})
					w.Write(queuedMsg)
				default:
					// No more messages ready, break out of batch loop
					break batchLoop
				}
			}

			// Close the writer to send the batched messages
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.connMu.Lock()
			conn := c.conn
			c.connMu.Unlock()

			if conn == nil {
				return
			}

			conn.SetWriteDeadline(time.Now().Add(agentWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// reconnectLoop attempts to reconnect when connection is lost
func (c *AgentWebSocketClient) reconnectLoop() {
	for c.reconnect {
		time.Sleep(reconnectDelay)
		
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		err := c.Connect(ctx)
		cancel()

		if err == nil {
			log.Printf("WebSocket reconnected successfully")
			return
		}
		log.Printf("WebSocket reconnect failed: %v, retrying in %v", err, reconnectDelay)
	}
}

