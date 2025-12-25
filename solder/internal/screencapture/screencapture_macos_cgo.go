//go:build darwin
// +build darwin

package screencapture

/*
#cgo darwin CFLAGS: -x objective-c -mmacosx-version-min=12.3
#cgo darwin LDFLAGS: -framework ScreenCaptureKit -framework AVFoundation -framework CoreVideo -framework CoreMedia -framework Foundation -framework CoreGraphics
#include "screencapture_macos.h"
#include <stdlib.h>
*/
import "C"
import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/image/draw"
)

// macosCaptureSession wraps the C ScreenCaptureKit session
type macosCaptureSession struct {
	session   unsafe.Pointer
	frameChan chan []byte
	stopChan  chan struct{}
	mu        sync.Mutex
	running   bool
	quality   int
	maxWidth  int
	maxHeight int
}

var (
	globalFrameHandler func(unsafe.Pointer, C.size_t, C.uint32_t, C.uint32_t, C.int64_t)
	globalFrameChan    chan []byte
	globalFrameMutex   sync.Mutex
)

// frameCallbackBridge is called from C when a frame is captured
//export frameCallbackBridge
func frameCallbackBridge(buffer unsafe.Pointer, size C.size_t, width C.uint32_t, height C.uint32_t, timestamp C.int64_t) {
	globalFrameMutex.Lock()
	handler := globalFrameHandler
	globalFrameMutex.Unlock()
	
	if handler != nil {
		handler(buffer, size, width, height, timestamp)
	} else {
		// No handler set, free the buffer
		C.free(buffer)
	}
}

func newMacOSCaptureSession(quality, maxWidth, maxHeight int) *macosCaptureSession {
	session := C.CreateCaptureSession()
	if session == nil {
		return nil
	}

	s := &macosCaptureSession{
		session:   session,
		frameChan: make(chan []byte, 10), // Buffer 10 frames for smoother streaming
		stopChan:  make(chan struct{}),
		quality:   quality,
		maxWidth:  maxWidth,
		maxHeight: maxHeight,
	}

	// Set up global callback handler
	globalFrameMutex.Lock()
	globalFrameChan = s.frameChan
	globalFrameHandler = func(buffer unsafe.Pointer, size C.size_t, width C.uint32_t, height C.uint32_t, timestamp C.int64_t) {
		// Copy C buffer to Go slice
		data := C.GoBytes(buffer, C.int(size))
		
		// Convert BGRA to RGBA and create image
		img := convertBGRAtoRGBA(data, int(width), int(height))
		
		// Resize if needed
		if s.maxWidth > 0 && s.maxHeight > 0 {
			img = resizeImage(img, s.maxWidth, s.maxHeight)
		}
		
		// Encode as JPEG
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: s.quality}); err != nil {
			return
		}
		
		// Send to channel (non-blocking)
		select {
		case s.frameChan <- buf.Bytes():
		default:
			// Channel full, drop frame
		}
		
		// Free C memory
		C.free(buffer)
	}
	globalFrameMutex.Unlock()

	// Set C callback - use a bridge function
	C.SetFrameCallback(session, (*[0]byte)(C.frameCallbackBridge))

	// Set finalizer for cleanup
	runtime.SetFinalizer(s, (*macosCaptureSession).cleanup)

	return s
}

func (s *macosCaptureSession) cleanup() {
	if s.session != nil {
		C.DestroyCaptureSession(s.session)
		s.session = nil
	}
}

func (s *macosCaptureSession) startCapture(displayID uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("capture already running")
	}

	result := C.StartCapture(s.session, C.uint32_t(displayID), C.int(s.maxWidth), C.int(s.maxHeight))
	if result == 0 {
		return fmt.Errorf("failed to start capture")
	}

	s.running = true
	return nil
}

func (s *macosCaptureSession) stopCapture() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	C.StopCapture(s.session)
	s.running = false
	
	// Close channel only if it's not nil (prevent double-close panic)
	if s.stopChan != nil {
		close(s.stopChan)
		s.stopChan = nil
	}
}

func (s *macosCaptureSession) isCapturing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running && C.IsCapturing(s.session) != 0
}

func (s *macosCaptureSession) getFrame(ctx context.Context) ([]byte, error) {
	select {
	case frame := <-s.frameChan:
		return frame, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.stopChan:
		return nil, fmt.Errorf("capture stopped")
	}
}

// convertBGRAtoRGBA converts BGRA pixel data to RGBA image
func convertBGRAtoRGBA(data []byte, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	
	// BGRA to RGBA conversion
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcIdx := (y*width + x) * 4
			dstIdx := img.PixOffset(x, y)
			
			// BGRA -> RGBA: swap R and B channels
			img.Pix[dstIdx+0] = data[srcIdx+2] // R
			img.Pix[dstIdx+1] = data[srcIdx+1] // G
			img.Pix[dstIdx+2] = data[srcIdx+0] // B
			img.Pix[dstIdx+3] = data[srcIdx+3] // A
		}
	}
	
	return img
}

// resizeImage resizes an image maintaining aspect ratio
func resizeImage(img image.Image, maxWidth, maxHeight int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Check if resize is needed
	if width <= maxWidth && height <= maxHeight {
		return img
	}

	// Calculate new dimensions maintaining aspect ratio
	ratio := float64(width) / float64(height)
	var newWidth, newHeight int

	if width > height {
		newWidth = maxWidth
		newHeight = int(float64(maxWidth) / ratio)
		if newHeight > maxHeight {
			newHeight = maxHeight
			newWidth = int(float64(maxHeight) * ratio)
		}
	} else {
		newHeight = maxHeight
		newWidth = int(float64(maxHeight) * ratio)
		if newWidth > maxWidth {
			newWidth = maxWidth
			newHeight = int(float64(maxWidth) / ratio)
		}
	}

	// Resize using high-quality resampling
	dst := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)

	return dst
}

// C wrapper functions for permission and display management

func hasScreenRecordingPermission() bool {
	return C.HasScreenRecordingPermission() != 0
}

func requestScreenRecordingPermission() {
	C.RequestScreenRecordingPermission()
}

func getPrimaryDisplayID() uint32 {
	return uint32(C.GetPrimaryDisplayID())
}

func getDisplayCount() int {
	return int(C.GetDisplayCount())
}

func getDisplayIDs() []uint32 {
	count := getDisplayCount()
	if count == 0 {
		return nil
	}
	
	displays := make([]C.uint32_t, count)
	C.GetDisplayIDs((*C.uint32_t)(&displays[0]), C.int(count))
	
	result := make([]uint32, count)
	for i := 0; i < count; i++ {
		result[i] = uint32(displays[i])
	}
	return result
}

