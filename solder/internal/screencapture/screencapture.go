//go:build !darwin
// +build !darwin

package screencapture

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/kbinani/screenshot"
	"golang.org/x/image/draw"
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
	selectedScreenIndex int
	mu                 sync.Mutex
}

func NewCaptureService(cfg config.ScreenCaptureConfig) *CaptureService {
	return &CaptureService{
		enabled:            cfg.Enabled && isDesktopAvailable(),
		interval:           time.Duration(cfg.IntervalSeconds * float64(time.Second)),
		quality:            cfg.Quality,
		maxWidth:           cfg.MaxWidth,
		maxHeight:          cfg.MaxHeight,
		stopChan:           make(chan struct{}),
		running:            false,
		selectedScreenIndex: 0, // Default to primary display
	}
}

func isDesktopAvailable() bool {
	if runtime.GOOS == "linux" {
		display := os.Getenv("DISPLAY")
		if display == "" {
			// Try Wayland
			if os.Getenv("WAYLAND_DISPLAY") == "" {
				return false
			}
		}
	}
	return true
}

func (s *CaptureService) CaptureScreen() ([]byte, error) {
	if !s.enabled {
		return nil, fmt.Errorf("screen capture not available on this system")
	}

	s.mu.Lock()
	screenIndex := s.selectedScreenIndex
	s.mu.Unlock()

	// Validate screen index
	numDisplays := screenshot.NumActiveDisplays()
	if screenIndex < 0 || screenIndex >= numDisplays {
		// Fallback to primary display if index is invalid
		screenIndex = 0
	}

	// Get display bounds for selected screen
	bounds := screenshot.GetDisplayBounds(screenIndex)

	// Capture screen
	capturedImg, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, fmt.Errorf("failed to capture screen: %w", err)
	}

	// Resize if needed
	var img image.Image = s.resizeImage(capturedImg)

	// Encode as JPEG with quality settings
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: s.quality}); err != nil {
		return nil, fmt.Errorf("failed to encode image: %w", err)
	}

	s.lastCapture = time.Now()
	return buf.Bytes(), nil
}

// ScreenInfo represents information about an available screen/display
type ScreenInfo struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	IsPrimary bool   `json:"is_primary"`
}

// GetAvailableScreens returns a list of available screens/displays
func (s *CaptureService) GetAvailableScreens() ([]ScreenInfo, error) {
	if !s.enabled {
		return nil, fmt.Errorf("screen capture not available on this system")
	}

	numDisplays := screenshot.NumActiveDisplays()
	screens := make([]ScreenInfo, 0, numDisplays)

	for i := 0; i < numDisplays; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		screens = append(screens, ScreenInfo{
			Index:     i,
			Name:      fmt.Sprintf("Display %d", i+1),
			Width:     bounds.Dx(),
			Height:    bounds.Dy(),
			IsPrimary: i == 0,
		})
	}

	return screens, nil
}

// SetScreenIndex sets the screen index to capture
func (s *CaptureService) SetScreenIndex(index int) error {
	if !s.enabled {
		return fmt.Errorf("screen capture not available on this system")
	}

	numDisplays := screenshot.NumActiveDisplays()
	if index < 0 || index >= numDisplays {
		return fmt.Errorf("invalid screen index %d, must be between 0 and %d", index, numDisplays-1)
	}

	s.mu.Lock()
	s.selectedScreenIndex = index
	s.mu.Unlock()

	return nil
}

func (s *CaptureService) resizeImage(img image.Image) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Check if resize is needed
	if width <= s.maxWidth && height <= s.maxHeight {
		return img
	}

	// Calculate new dimensions maintaining aspect ratio
	ratio := float64(width) / float64(height)
	var newWidth, newHeight int

	if width > height {
		newWidth = s.maxWidth
		newHeight = int(float64(s.maxWidth) / ratio)
		if newHeight > s.maxHeight {
			newHeight = s.maxHeight
			newWidth = int(float64(s.maxHeight) * ratio)
		}
	} else {
		newHeight = s.maxHeight
		newWidth = int(float64(s.maxHeight) * ratio)
		if newWidth > s.maxWidth {
			newWidth = s.maxWidth
			newHeight = int(float64(s.maxWidth) / ratio)
		}
	}

	// Resize using high-quality resampling
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)

	return dst
}

// Start starts continuous capture (legacy method)
func (s *CaptureService) Start(ctx context.Context, captureFunc func([]byte) error) {
	if !s.enabled {
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := s.CaptureScreen()
			if err != nil {
				// Log error but continue
				continue
			}
			if err := captureFunc(data); err != nil {
				// Log error but continue
				continue
			}
		}
	}
}

// StartStreaming starts on-demand capture streaming with frame rate limiting
func (s *CaptureService) StartStreaming(ctx context.Context, captureFunc func([]byte) error) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("capture service is already running")
	}
	if !s.enabled {
		s.mu.Unlock()
		return fmt.Errorf("screen capture not enabled")
	}
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	// Get current interval (will use settings from mothership if updated)
	s.mu.Lock()
	streamInterval := s.interval
	s.mu.Unlock()
	
	// Cap at reasonable limits
	if streamInterval < 100*time.Millisecond {
		streamInterval = 100 * time.Millisecond // Max 10 FPS
	}
	if streamInterval > 2*time.Second {
		streamInterval = 2 * time.Second // Min 0.5 FPS
	}

	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()

	// Frame queue to prevent blocking and allow backpressure
	frameChan := make(chan []byte, 2) // Buffer max 2 frames
	
	// Frame sender goroutine with backpressure
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopChan:
				return
			case frameData := <-frameChan:
				// Send frame, if it fails, skip this frame (non-blocking)
				if err := captureFunc(frameData); err != nil {
					// Log error but continue - drop frame if send fails
					continue
				}
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.running = false
			close(frameChan)
			s.mu.Unlock()
			return nil
		case <-s.stopChan:
			s.mu.Lock()
			s.running = false
			close(frameChan)
			s.mu.Unlock()
			return nil
		case <-ticker.C:
			// Check if interval changed and update ticker if needed
			s.mu.Lock()
			currentInterval := s.interval
			s.mu.Unlock()
			
			if currentInterval != streamInterval {
				// Interval changed, update ticker
				if currentInterval < 100*time.Millisecond {
					currentInterval = 100 * time.Millisecond
				}
				if currentInterval > 2*time.Second {
					currentInterval = 2 * time.Second
				}
				ticker.Stop()
				ticker = time.NewTicker(currentInterval)
				streamInterval = currentInterval
			}
			data, err := s.CaptureScreen()
			if err != nil {
				// Log error but continue
				continue
			}
			
			// Non-blocking send to frame channel
			select {
			case frameChan <- data:
				// Frame queued successfully
			default:
				// Channel full, skip this frame to maintain real-time performance
				// This prevents buffer buildup and keeps the stream responsive
			}
		}
	}
}

// StopStreaming stops the capture streaming
func (s *CaptureService) StopStreaming() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		// Close channel only if it's not nil (prevent double-close panic)
		if s.stopChan != nil {
			close(s.stopChan)
			s.stopChan = nil
		}
		s.running = false
	}
}

// IsRunning returns whether capture is currently running
func (s *CaptureService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *CaptureService) IsEnabled() bool {
	return s.enabled
}

// UpdateSettings updates quality and FPS settings dynamically
// This can be called while streaming is active to update settings on the fly
func (s *CaptureService) UpdateSettings(quality int, fps float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if quality > 0 && quality <= 100 {
		s.quality = quality
	}
	
	if fps > 0 && fps <= 10 {
		// Convert FPS to interval duration
		newInterval := time.Duration(float64(time.Second) / fps)
		oldInterval := s.interval
		s.interval = newInterval
		
		// If streaming is active and interval changed, we need to restart the ticker
		// This is handled by checking interval in the main loop
		_ = oldInterval // Avoid unused variable warning
	}
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

