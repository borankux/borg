#ifndef SCREENCAPTURE_MACOS_H
#define SCREENCAPTURE_MACOS_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// Frame callback function pointer type
typedef void (*FrameCallback)(void* buffer, size_t size, uint32_t width, uint32_t height, int64_t timestamp);

// Bridge function exported from Go
extern void frameCallbackBridge(void* buffer, size_t size, uint32_t width, uint32_t height, int64_t timestamp);

// Initialization
void* CreateCaptureSession(void);
void DestroyCaptureSession(void* session);

// Capture control
int StartCapture(void* session, uint32_t displayID, int maxWidth, int maxHeight);
void StopCapture(void* session);
int IsCapturing(void* session);

// Frame callback registration
void SetFrameCallback(void* session, FrameCallback callback);

// Permission check
int HasScreenRecordingPermission(void);
void RequestScreenRecordingPermission(void);

// Display enumeration
uint32_t GetPrimaryDisplayID(void);
int GetDisplayCount(void);
void GetDisplayIDs(uint32_t* displays, int count);

#ifdef __cplusplus
}
#endif

#endif // SCREENCAPTURE_MACOS_H

