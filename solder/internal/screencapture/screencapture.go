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
	enabled     bool
	interval    time.Duration
	quality     int
	maxWidth    int
	maxHeight   int
	lastCapture time.Time
	stopChan    chan struct{}
	running     bool
	mu          sync.Mutex
}

func NewCaptureService(cfg config.ScreenCaptureConfig) *CaptureService {
	return &CaptureService{
		enabled:  cfg.Enabled && isDesktopAvailable(),
		interval: time.Duration(cfg.IntervalSeconds) * time.Second,
		quality:  cfg.Quality,
		maxWidth: cfg.MaxWidth,
		maxHeight: cfg.MaxHeight,
		stopChan: make(chan struct{}),
		running:  false,
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

	// Get primary display bounds
	bounds := screenshot.GetDisplayBounds(0)

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

// StartStreaming starts on-demand capture streaming
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

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return nil
		case <-s.stopChan:
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			return nil
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

// StopStreaming stops the capture streaming
func (s *CaptureService) StopStreaming() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		close(s.stopChan)
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

