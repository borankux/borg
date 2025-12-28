//go:build darwin
// +build darwin

package screencapture

/*
#cgo darwin CFLAGS: -x objective-c -mmacosx-version-min=12.3
#cgo darwin LDFLAGS: -framework ScreenCaptureKit -framework AVFoundation -framework CoreVideo -framework CoreMedia -framework Foundation -framework CoreGraphics
#include "screencapture_macos.h"
#include <CoreGraphics/CoreGraphics.h>
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

// rawFrame represents an unprocessed frame from capture
type rawFrame struct {
	data   []byte
	width  int
	height int
}

// macosCaptureSession wraps the C ScreenCaptureKit session
type macosCaptureSession struct {
	session     unsafe.Pointer
	frameChan   chan []byte      // Processed JPEG frames
	rawFrameChan chan rawFrame    // Raw BGRA frames for processing
	stopChan    chan struct{}
	mu          sync.Mutex
	running     bool
	quality     int
	maxWidth    int
	maxHeight   int
	processingWg sync.WaitGroup  // Wait for processing goroutine
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
		session:      session,
		frameChan:    make(chan []byte, 10),   // Processed frames (larger buffer for bursts)
		rawFrameChan: make(chan rawFrame, 5),  // Raw frames (larger buffer, drops old frames)
		stopChan:     make(chan struct{}),
		quality:      quality,
		maxWidth:     maxWidth,
		maxHeight:    maxHeight,
	}

	// Set up global callback handler - fast path: just copy and queue
	globalFrameMutex.Lock()
	globalFrameChan = s.frameChan
	globalFrameHandler = func(buffer unsafe.Pointer, size C.size_t, width C.uint32_t, height C.uint32_t, timestamp C.int64_t) {
		// Fast: Copy C buffer to Go slice
		data := C.GoBytes(buffer, C.int(size))
		
		// Free C memory immediately (we have Go copy)
		C.free(buffer)
		
		// Queue raw frame for processing (non-blocking, drops if full)
		select {
		case s.rawFrameChan <- rawFrame{data: data, width: int(width), height: int(height)}:
		default:
			// Channel full - drop oldest frame and queue new one (frame skipping)
			select {
			case <-s.rawFrameChan:
				// Dropped old frame, now try again
				select {
				case s.rawFrameChan <- rawFrame{data: data, width: int(width), height: int(height)}:
				default:
					// Still full, drop this frame too
				}
			default:
				// Nothing to drop, skip this frame
			}
		}
	}
	globalFrameMutex.Unlock()
	
	// Start processing goroutine (non-blocking)
	s.processingWg.Add(1)
	go s.processFrames()

	// Set C callback - use a bridge function
	C.SetFrameCallback(session, (*[0]byte)(C.frameCallbackBridge))

	// Set finalizer for cleanup
	runtime.SetFinalizer(s, (*macosCaptureSession).cleanup)

	return s
}

func (s *macosCaptureSession) cleanup() {
	if s.session != nil {
		// Stop capture first
		s.stopCapture()
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

// processFrames processes raw frames in a separate goroutine (non-blocking)
func (s *macosCaptureSession) processFrames() {
	defer s.processingWg.Done()
	
	for {
		select {
		case <-s.stopChan:
			return
		case rawFrame, ok := <-s.rawFrameChan:
			if !ok {
				// Channel closed
				return
			}
			
			// Convert BGRA to RGBA (optimized)
			img := convertBGRAtoRGBAFast(rawFrame.data, rawFrame.width, rawFrame.height)
			
			// Resize if needed
			if s.maxWidth > 0 && s.maxHeight > 0 {
				img = resizeImage(img, s.maxWidth, s.maxHeight)
			}
			
			// Encode as JPEG
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: s.quality}); err != nil {
				continue
			}
			
			// Send processed frame (non-blocking, drops if full)
			select {
			case s.frameChan <- buf.Bytes():
			default:
				// Channel full, drop frame
			}
		}
	}
}

func (s *macosCaptureSession) stopCapture() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	C.StopCapture(s.session)
	s.running = false
	
	// Signal processing goroutine to stop
	if s.stopChan != nil {
		close(s.stopChan)
		s.stopChan = nil
	}
	
	// Close raw frame channel to signal processing goroutine
	if s.rawFrameChan != nil {
		close(s.rawFrameChan)
	}
	
	// Wait for processing goroutine to finish
	s.processingWg.Wait()
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

// GetFrameChannel returns the frame channel for event-driven processing
func (s *macosCaptureSession) GetFrameChannel() <-chan []byte {
	return s.frameChan
}

// convertBGRAtoRGBAFast converts BGRA pixel data to RGBA image (optimized)
// Uses single loop instead of nested loops for better performance
func convertBGRAtoRGBAFast(data []byte, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	pixels := width * height
	
	// Optimized: single loop, direct indexing (much faster than nested loops)
	// Process all pixels in one pass for better CPU cache usage
	for i := 0; i < pixels; i++ {
		srcIdx := i * 4
		dstIdx := i * 4
		
		// BGRA -> RGBA: swap R and B channels
		img.Pix[dstIdx+0] = data[srcIdx+2] // R
		img.Pix[dstIdx+1] = data[srcIdx+1] // G
		img.Pix[dstIdx+2] = data[srcIdx+0] // B
		img.Pix[dstIdx+3] = data[srcIdx+3] // A
	}
	
	return img
}

// convertBGRAtoRGBA is kept for backward compatibility (deprecated)
func convertBGRAtoRGBA(data []byte, width, height int) image.Image {
	return convertBGRAtoRGBAFast(data, width, height)
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

func getDisplayWidth(displayID uint32) uint32 {
	return uint32(C.CGDisplayPixelsWide(C.uint32_t(displayID)))
}

func getDisplayWidth(displayID uint32) uint32 {
	return uint32(C.CGDisplayPixelsWide(C.uint32_t(displayID)))
}

func getDisplayHeight(displayID uint32) uint32 {
	return uint32(C.CGDisplayPixelsHigh(C.uint32_t(displayID)))
}

