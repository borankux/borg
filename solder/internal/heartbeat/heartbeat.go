package heartbeat

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"borg/solder/internal/client"
)

// Heartbeat manages heartbeat to mothership
type Heartbeat struct {
	client      *client.Client
	runnerID    string
	interval    time.Duration
	activeTasks int32
	status      string // idle, busy, offline
	stopChan    chan struct{}
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
	
	resp, err := h.client.Heartbeat(ctx, h.status, activeTasks)
	if err != nil {
		log.Printf("Failed to send heartbeat: %v", err)
		return
	}
	
	if resp.Success && resp.NextHeartbeatInterval > 0 {
		// Update interval if mothership requests different interval
		newInterval := time.Duration(resp.NextHeartbeatInterval) * time.Second
		if newInterval != h.interval {
			log.Printf("Updating heartbeat interval to %v", newInterval)
			h.interval = newInterval
		}
	}
}

