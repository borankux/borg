//go:build darwin
// +build darwin

package screencapture

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"borg/solder/internal/config"
)

type CaptureService struct {
	enabled     bool
	interval    time.Duration
	quality     int
	maxWidth    int
	maxHeight   int
	lastCapture time.Time
	stopChan    chan struct{}
	running     bool
	mu          sync.Mutex
	session     *macosCaptureSession
	displayID   uint32
}

func NewCaptureService(cfg config.ScreenCaptureConfig) *CaptureService {
	enabled := cfg.Enabled && isDesktopAvailable()
	
	service := &CaptureService{
		enabled:  enabled,
		interval: time.Duration(cfg.IntervalSeconds * float64(time.Second)),
		quality:  cfg.Quality,
		maxWidth: cfg.MaxWidth,
		maxHeight: cfg.MaxHeight,
		stopChan: make(chan struct{}),
		running:  false,
	}
	
	if enabled {
		// Get primary display ID
		service.displayID = getPrimaryDisplayID()
		
		// Create capture session
		service.session = newMacOSCaptureSession(cfg.Quality, cfg.MaxWidth, cfg.MaxHeight)
		if service.session == nil {
			log.Println("Failed to create macOS capture session")
			service.enabled = false
		}
	}
	
	return service
}

func isDesktopAvailable() bool {
	// Check if ScreenCaptureKit is available (macOS 12.3+)
	// Note: Permission check may trigger system dialog on first attempt
	if !hasScreenRecordingPermission() {
		// Don't log here - let the caller handle logging with more context
		return false
	}
	return true
}

func (s *CaptureService) ensurePermission() error {
	if !hasScreenRecordingPermission() {
		log.Println("Screen Recording permission required. Attempting to request permission...")
		requestScreenRecordingPermission()
		// Give system a moment to show permission dialog
		time.Sleep(500 * time.Millisecond)
		
		if !hasScreenRecordingPermission() {
			return fmt.Errorf("screen recording permission not granted. Please grant permission in System Settings > Privacy & Security > Screen Recording")
		}
	}
	return nil
}

func (s *CaptureService) CaptureScreen() ([]byte, error) {
	if !s.enabled {
		return nil, fmt.Errorf("screen capture not available on this system")
	}
	
	if err := s.ensurePermission(); err != nil {
		return nil, err
	}
	
	if s.session == nil {
		return nil, fmt.Errorf("capture session not initialized")
	}
	
	// Start capture if not already running
	s.mu.Lock()
	wasRunning := s.running
	if !s.running {
		if err := s.session.startCapture(s.displayID); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("failed to start capture: %w", err)
		}
		s.running = true
	}
	s.mu.Unlock()
	
	// Get a single frame with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	frame, err := s.session.getFrame(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to capture frame: %w", err)
	}
	
	// Stop capture if we started it
	if !wasRunning {
		s.mu.Lock()
		s.session.stopCapture()
		s.running = false
		s.mu.Unlock()
	}
	
	s.lastCapture = time.Now()
	return frame, nil
}

func (s *CaptureService) Start(ctx context.Context, captureFunc func([]byte) error) {
	if !s.enabled {
		return
	}
	
	// Legacy method - use StartStreaming instead
	if err := s.StartStreaming(ctx, captureFunc); err != nil {
		log.Printf("Failed to start streaming: %v", err)
	}
}

func (s *CaptureService) StartStreaming(ctx context.Context, captureFunc func([]byte) error) error {
	if !s.enabled {
		return fmt.Errorf("screen capture not enabled")
	}
	
	if err := s.ensurePermission(); err != nil {
		return err
	}
	
	if s.session == nil {
		return fmt.Errorf("capture session not initialized")
	}
	
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("capture already running")
	}
	
	if err := s.session.startCapture(s.displayID); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to start capture: %w", err)
	}
	
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()
	
	// Start frame processing goroutine - process frames as they arrive (event-driven, no ticker)
	go func() {
		// Rate limiter: ensure we don't send frames faster than interval
		var lastSendTime time.Time
		minInterval := s.interval
		frameChan := s.session.GetFrameChannel()
		
		for {
			select {
			case <-ctx.Done():
				s.StopStreaming()
				return
			case <-s.stopChan:
				return
			case frame := <-frameChan:
				// Rate limiting: ensure minimum interval between sends
				now := time.Now()
				if !lastSendTime.IsZero() && now.Sub(lastSendTime) < minInterval {
					// Skip frame if sending too fast (drop frame)
					continue
				}
				
				// Send frame asynchronously to avoid blocking
				go func(f []byte) {
					if err := captureFunc(f); err != nil {
						log.Printf("Failed to send frame: %v", err)
					}
				}(frame)
				
				lastSendTime = now
				s.lastCapture = now
			}
		}
	}()
	
	return nil
}

func (s *CaptureService) StopStreaming() {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if !s.running {
		return
	}
	
	if s.session != nil {
		s.session.stopCapture()
	}
	
	s.running = false
	
	// Close channel only if it's not nil (prevent double-close panic)
	if s.stopChan != nil {
		close(s.stopChan)
		s.stopChan = nil
	}
}

func (s *CaptureService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running && s.session != nil && s.session.isCapturing()
}

func (s *CaptureService) IsEnabled() bool {
	return s.enabled
}

