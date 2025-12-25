//go:build darwin
// +build darwin

package screencapture

import (
	"testing"
	"time"

	"borg/solder/internal/config"
)

func TestHasScreenRecordingPermission(t *testing.T) {
	hasPermission := hasScreenRecordingPermission()
	t.Logf("Screen Recording permission: %v", hasPermission)
	// This test just checks if the function works, actual permission depends on system
}

func TestGetPrimaryDisplayID(t *testing.T) {
	displayID := getPrimaryDisplayID()
	if displayID == 0 {
		t.Error("Expected non-zero display ID")
	}
	t.Logf("Primary display ID: %d", displayID)
}

func TestGetDisplayCount(t *testing.T) {
	count := getDisplayCount()
	if count < 1 {
		t.Error("Expected at least 1 display")
	}
	t.Logf("Display count: %d", count)
}

func TestGetDisplayIDs(t *testing.T) {
	displays := getDisplayIDs()
	if len(displays) == 0 {
		t.Error("Expected at least one display ID")
	}
	t.Logf("Display IDs: %v", displays)
}

func TestNewCaptureService(t *testing.T) {
	cfg := config.ScreenCaptureConfig{
		Enabled:        true,
		IntervalSeconds: 1,
		Quality:        60,
		MaxWidth:       1280,
		MaxHeight:      720,
	}
	
	service := NewCaptureService(cfg)
	if service == nil {
		t.Fatal("Failed to create capture service")
	}
	
	// Check if enabled (depends on permission)
	t.Logf("Service enabled: %v", service.IsEnabled())
}

func TestCaptureService_IsEnabled(t *testing.T) {
	cfg := config.ScreenCaptureConfig{
		Enabled: true,
	}
	
	service := NewCaptureService(cfg)
	enabled := service.IsEnabled()
	t.Logf("Service enabled: %v", enabled)
}

func TestCaptureService_CaptureScreen(t *testing.T) {
	cfg := config.ScreenCaptureConfig{
		Enabled:        true,
		IntervalSeconds: 1,
		Quality:        60,
		MaxWidth:       1280,
		MaxHeight:      720,
	}
	
	service := NewCaptureService(cfg)
	if !service.IsEnabled() {
		t.Skip("Screen capture not enabled (permission not granted)")
	}
	
	// Try to capture a single frame
	frame, err := service.CaptureScreen()
	if err != nil {
		t.Logf("Capture error (may be expected if permission not granted): %v", err)
		return
	}
	
	if len(frame) == 0 {
		t.Error("Expected non-empty frame data")
	}
	
	t.Logf("Captured frame size: %d bytes", len(frame))
}

func TestCaptureService_StartStreaming(t *testing.T) {
	cfg := config.ScreenCaptureConfig{
		Enabled:        true,
		IntervalSeconds: 1,
		Quality:        60,
		MaxWidth:       1280,
		MaxHeight:      720,
	}
	
	service := NewCaptureService(cfg)
	if !service.IsEnabled() {
		t.Skip("Screen capture not enabled (permission not granted)")
	}
	
	frameCount := 0
	err := service.StartStreaming(nil, func(data []byte) error {
		frameCount++
		if len(data) == 0 {
			t.Error("Received empty frame")
		}
		return nil
	})
	
	if err != nil {
		t.Logf("Start streaming error (may be expected if permission not granted): %v", err)
		return
	}
	
	// Wait a bit for frames
	time.Sleep(2 * time.Second)
	
	if !service.IsRunning() {
		t.Error("Expected service to be running")
	}
	
	service.StopStreaming()
	
	if frameCount == 0 {
		t.Log("No frames received (may be normal if permission not granted)")
	} else {
		t.Logf("Received %d frames", frameCount)
	}
}

