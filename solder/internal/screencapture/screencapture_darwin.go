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
	enabled            bool
	interval           time.Duration
	quality            int
	maxWidth           int
	maxHeight          int
	lastCapture        time.Time
	stopChan           chan struct{}
	running            bool
	mu                 sync.Mutex
	session            *macosCaptureSession
	displayID          uint32
	selectedScreenIndex int
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
		service.selectedScreenIndex = 0
		
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

	s.mu.Lock()
	screenIndex := s.selectedScreenIndex
	s.mu.Unlock()

	// Get display IDs and validate index
	displayIDs := getDisplayIDs()
	if screenIndex < 0 || screenIndex >= len(displayIDs) {
		screenIndex = 0 // Fallback to primary
	}
	displayID := displayIDs[screenIndex]

	// Update displayID if screen index changed
	s.mu.Lock()
	if s.displayID != displayID {
		s.displayID = displayID
	}
	wasRunning := s.running
	if !wasRunning {
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

	// Ensure displayID matches selectedScreenIndex
	screenIndex := s.selectedScreenIndex
	displayIDs := getDisplayIDs()
	if screenIndex < 0 || screenIndex >= len(displayIDs) {
		screenIndex = 0
	}
	s.displayID = displayIDs[screenIndex]
	s.selectedScreenIndex = screenIndex
	
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

// ScreenInfo represents information about an available screen/display
type ScreenInfo struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	IsPrimary bool   `json:"is_primary"`
}

// GetQuality returns the current quality setting
func (s *CaptureService) GetQuality() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.quality
}

// GetInterval returns the current capture interval
func (s *CaptureService) GetInterval() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

// UpdateSettings updates quality and FPS settings dynamically
func (s *CaptureService) UpdateSettings(quality int, fps float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if quality > 0 && quality <= 100 {
		s.quality = quality
		if s.session != nil {
			// Update session quality if possible
			// Note: macOS capture session quality might need to be updated differently
		}
	}
	
	if fps > 0 && fps <= 10 {
		// Convert FPS to interval duration
		s.interval = time.Duration(float64(time.Second) / fps)
	}
}

// GetAvailableScreens returns a list of available screens/displays (macOS)
func (s *CaptureService) GetAvailableScreens() ([]ScreenInfo, error) {
	if !s.enabled {
		return nil, fmt.Errorf("screen capture not available on this system")
	}

	displayIDs := getDisplayIDs()
	screens := make([]ScreenInfo, 0, len(displayIDs))

	for i, displayID := range displayIDs {
		// Get display dimensions using CGO wrapper functions
		width := int(getDisplayWidth(displayID))
		height := int(getDisplayHeight(displayID))
		
		screens = append(screens, ScreenInfo{
			Index:     i,
			Name:      fmt.Sprintf("Display %d", i+1),
			Width:     width,
			Height:    height,
			IsPrimary: i == 0,
		})
	}

	return screens, nil
}

// SetScreenIndex sets the screen index to capture (macOS)
func (s *CaptureService) SetScreenIndex(index int) error {
	if !s.enabled {
		return fmt.Errorf("screen capture not available on this system")
	}

	displayIDs := getDisplayIDs()
	if index < 0 || index >= len(displayIDs) {
		return fmt.Errorf("invalid screen index %d, must be between 0 and %d", index, len(displayIDs)-1)
	}

	s.mu.Lock()
	oldDisplayID := s.displayID
	s.selectedScreenIndex = index
	newDisplayID := displayIDs[index]
	s.displayID = newDisplayID
	wasRunning := s.running
	s.mu.Unlock()

	// If capture is running, restart with new display
	if wasRunning && s.session != nil && oldDisplayID != newDisplayID {
		s.session.stopCapture()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		if err := s.session.startCapture(newDisplayID); err != nil {
			return fmt.Errorf("failed to switch to display %d: %w", index, err)
		}
		s.mu.Lock()
		s.running = true
		s.mu.Unlock()
	}

	return nil
}

