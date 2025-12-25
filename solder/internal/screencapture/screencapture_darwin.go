//go:build darwin
// +build darwin

package screencapture

import (
	"context"
	"fmt"
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
}

func NewCaptureService(cfg config.ScreenCaptureConfig) *CaptureService {
	return &CaptureService{
		enabled:  false, // Always disabled on macOS due to API deprecation
		interval: time.Duration(cfg.IntervalSeconds) * time.Second,
		quality:  cfg.Quality,
		maxWidth: cfg.MaxWidth,
		maxHeight: cfg.MaxHeight,
		stopChan: make(chan struct{}),
		running:  false,
	}
}

func isDesktopAvailable() bool {
	// Screen capture is disabled on macOS due to deprecated APIs
	// The kbinani/screenshot library uses CGDisplayCreateImageForRect which is
	// obsoleted in macOS 15.0. ScreenCaptureKit would be needed but requires
	// Objective-C/Swift bindings which are complex to implement in Go.
	return false
}

func (s *CaptureService) CaptureScreen() ([]byte, error) {
	return nil, fmt.Errorf("screen capture is not supported on macOS due to API deprecation (macOS 15+). Use Windows or Linux for screen monitoring")
}

func (s *CaptureService) Start(ctx context.Context, captureFunc func([]byte) error) {
	// No-op on macOS - screen capture is disabled
	return
}

func (s *CaptureService) StartStreaming(ctx context.Context, captureFunc func([]byte) error) error {
	return fmt.Errorf("screen capture is not supported on macOS")
}

func (s *CaptureService) StopStreaming() {
	// No-op on macOS
}

func (s *CaptureService) IsRunning() bool {
	return false
}

func (s *CaptureService) IsEnabled() bool {
	return false // Always disabled on macOS
}

