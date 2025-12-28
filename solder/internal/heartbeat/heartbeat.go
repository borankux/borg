package heartbeat

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"borg/solder/internal/client"
	"borg/solder/internal/resources"
)

// Heartbeat manages heartbeat to mothership
type Heartbeat struct {
	client           *client.Client
	runnerID         string
	interval         time.Duration
	activeTasks      int32
	status           string // idle, busy, offline
	stopChan         chan struct{}
	resourceSyncCount int32 // Counter for periodic resource sync
}

// NewHeartbeat creates a new heartbeat manager
func NewHeartbeat(c *client.Client, runnerID string, interval time.Duration) *Heartbeat {
	return &Heartbeat{
		client:      c,
		runnerID:    runnerID,
		interval:    interval,
		status:      "idle",
		stopChan:    make(chan struct{}),
	}
}

// Start starts the heartbeat loop
func (h *Heartbeat) Start(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()
	
	// Send initial heartbeat
	h.sendHeartbeat(ctx)
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopChan:
			return
		case <-ticker.C:
			h.sendHeartbeat(ctx)
		}
	}
}

// Stop stops the heartbeat loop
func (h *Heartbeat) Stop() {
	close(h.stopChan)
}

// SetActiveTasks sets the number of active tasks
func (h *Heartbeat) SetActiveTasks(count int32) {
	atomic.StoreInt32(&h.activeTasks, count)
	if count > 0 {
		h.SetStatus("busy")
	} else {
		h.SetStatus("idle")
	}
}

// SetStatus sets the runner status
func (h *Heartbeat) SetStatus(status string) {
	h.status = status
}

// sendHeartbeat sends a heartbeat to mothership
func (h *Heartbeat) sendHeartbeat(ctx context.Context) {
	activeTasks := atomic.LoadInt32(&h.activeTasks)
	
	// Sync resources every 10 heartbeats (approximately every 5 minutes with 30s interval)
	shouldSyncResources := false
	count := atomic.AddInt32(&h.resourceSyncCount, 1)
	if count >= 10 {
		atomic.StoreInt32(&h.resourceSyncCount, 0)
		shouldSyncResources = true
	}
	
	var resourceUpdate *client.ResourceUpdate
	if shouldSyncResources {
		// Detect current resources
		req := &client.RegisterRunnerRequest{}
		if err := resources.FillResources(ctx, req); err == nil {
			resourceUpdate = &client.ResourceUpdate{
				DiskSpaceGB:      req.DiskSpaceGB,
				TotalDiskSpaceGB: req.TotalDiskSpaceGB,
				MemoryGB:         req.MemoryGB,
				PublicIPs:         req.PublicIPs,
			}
			log.Printf("Syncing resources: Disk %.2f/%.2f GB, Memory %.2f GB", 
				req.DiskSpaceGB, req.TotalDiskSpaceGB, req.MemoryGB)
		}
	}
	
	// Try WebSocket first, fallback to HTTP
	var resp *client.HeartbeatResponse
	var err error
	
	if h.client.IsAgentWebSocketConnected() {
		err = h.client.SendHeartbeatWebSocket(ctx, h.status, activeTasks, resourceUpdate)
		if err != nil {
			// WebSocket failed, fallback to HTTP
			log.Printf("WebSocket heartbeat failed: %v, falling back to HTTP", err)
			resp, err = h.client.Heartbeat(ctx, h.status, activeTasks, resourceUpdate)
		} else {
			// WebSocket success - use default interval (response not available via WebSocket)
			// For now, keep current interval. In future, we could add response handling
			return
		}
	} else {
		// Use HTTP
		resp, err = h.client.Heartbeat(ctx, h.status, activeTasks, resourceUpdate)
	}
	
	if err != nil {
		log.Printf("Failed to send heartbeat: %v", err)
		return
	}
	
	if resp != nil && resp.Success && resp.NextHeartbeatInterval > 0 {
		// Update interval if mothership requests different interval
		newInterval := time.Duration(resp.NextHeartbeatInterval) * time.Second
		if newInterval != h.interval {
			log.Printf("Updating heartbeat interval to %v", newInterval)
			h.interval = newInterval
		}
	}
}

