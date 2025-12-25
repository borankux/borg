package websocket

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Hub maintains the set of active clients and broadcasts messages to clients
type Hub struct {
	// Registered clients
	clients map[*Client]bool
	
	// Inbound messages from clients
	broadcast chan []byte
	
	// Register requests from clients
	register chan *Client
	
	// Unregister requests from clients
	unregister chan *Client
	
	mu sync.RWMutex
}

// NewHub creates a new hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run runs the hub
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Printf("Client connected. Total clients: %d", len(h.clients))
			
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			log.Printf("Client disconnected. Total clients: %d", len(h.clients))
			
		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends a message to all connected clients
func (h *Hub) Broadcast(message interface{}) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error marshaling message: %v", err)
		return
	}
	
	select {
	case h.broadcast <- data:
	default:
		log.Println("Broadcast channel full, dropping message")
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Client represents a websocket connection
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

const (
	// Time allowed to write a message to the peer
	writeWait = 10
	// Time allowed to read the next pong message from the peer
	pongWait = 60
	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer
	maxMessageSize = 512 * 1024
)

// NewClient creates a new client
func NewClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
	}
}

// ReadPump pumps messages from the websocket connection to the hub
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	
	c.conn.SetReadDeadline(time.Now().Add(pongWait * time.Second))
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait * time.Second))
		return nil
	})
	
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

// WritePump pumps messages from the hub to the websocket connection
func (c *Client) WritePump() {
	defer c.conn.Close()
	
	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)
			
			// Add queued messages to current websocket message
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}
			
			if err := w.Close(); err != nil {
				return
			}
		}
	}
}

// Message represents a WebSocket message
type Message struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// NewMessage creates a new message
func NewMessage(messageType string, data interface{}) *Message {
	return &Message{
		Type: messageType,
		Data: data,
	}
}

