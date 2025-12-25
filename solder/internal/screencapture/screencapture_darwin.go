//go:build darwin
// +build darwin

package screencapture

import (
	"context"
	"fmt"
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
}

func NewCaptureService(cfg config.ScreenCaptureConfig) *CaptureService {
	return &CaptureService{
		enabled:  false, // Always disabled on macOS due to API deprecation
		interval: time.Duration(cfg.IntervalSeconds) * time.Second,
		quality:  cfg.Quality,
		maxWidth: cfg.MaxWidth,
		maxHeight: cfg.MaxHeight,
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

func (s *CaptureService) IsEnabled() bool {
	return false // Always disabled on macOS
}

