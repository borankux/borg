# macOS Screen Capture Implementation

This directory contains the macOS-specific screen capture implementation using Apple's ScreenCaptureKit framework via CGO.

## Requirements

- **macOS**: 12.3 or later (ScreenCaptureKit availability)
- **Xcode**: Command Line Tools installed
- **CGO**: Enabled (`CGO_ENABLED=1`)
- **Permission**: Screen Recording permission must be granted

## Architecture

The implementation consists of three main components:

1. **Objective-C Wrapper** (`screencapture_macos.m`): Uses ScreenCaptureKit to capture screen content
2. **C Bridge Header** (`screencapture_macos.h`): C interface for Go to call Objective-C code
3. **CGO Bridge** (`screencapture_macos_cgo.go`): Go wrapper that bridges C and Go, handles frame conversion

## Building

The build system automatically detects macOS and includes the necessary CGO flags:

```bash
cd solder
go build ./cmd/agent
```

CGO is required for macOS builds. Ensure `CGO_ENABLED=1` (this is the default).

## Permissions

### Granting Screen Recording Permission

1. Open **System Settings** (or **System Preferences** on older macOS)
2. Navigate to **Privacy & Security** → **Screen Recording**
3. Click the **+** button
4. Navigate to your terminal application (Terminal, iTerm, etc.) or the compiled `solder` binary
5. Enable the checkbox for the application

Alternatively, when you first run the agent, macOS may prompt you automatically. If you see a permission dialog, click **Open System Settings** and grant permission.

### Checking Permission Status

The agent will check for permission on startup. If permission is not granted, you'll see:

```
Screen Recording permission not granted. Please grant permission in System Settings > Privacy & Security > Screen Recording
```

### Troubleshooting Permission Issues

- **Permission denied errors**: Ensure the terminal/application has Screen Recording permission
- **No permission dialog**: Try running the agent once, then check System Settings manually
- **Permission granted but still fails**: Restart the agent after granting permission

## Usage

The macOS implementation works the same as other platforms:

```go
cfg := config.ScreenCaptureConfig{
    Enabled:        true,
    IntervalSeconds: 1,
    Quality:        60,
    MaxWidth:       1280,
    MaxHeight:      720,
}

service := screencapture.NewCaptureService(cfg)
if service.IsEnabled() {
    // Start streaming
    service.StartStreaming(ctx, func(frame []byte) error {
        // Process frame
        return nil
    })
}
```

## Technical Details

### Frame Format

- **Capture Format**: BGRA (32-bit per pixel)
- **Output Format**: JPEG (configurable quality)
- **Resolution**: Configurable max width/height (maintains aspect ratio)

### Performance

- **Target FPS**: Up to 30 FPS (configurable via interval)
- **Memory**: Frames are buffered (3 frame buffer) to prevent blocking
- **CPU**: Frame conversion and JPEG encoding happen in Go

### Thread Safety

- ScreenCaptureKit callbacks run on background threads
- Frames are passed to Go via channels (thread-safe)
- All Go operations are synchronized with mutexes

## Limitations

- **macOS Version**: Requires macOS 12.3+ (ScreenCaptureKit)
- **Permission**: Requires explicit user permission
- **Display**: Currently captures primary display only
- **Build**: Requires native macOS build (CGO doesn't cross-compile easily)

## Error Handling

Common errors and solutions:

- **"screen recording permission not granted"**: Grant permission in System Settings
- **"failed to start capture"**: Check permission, ensure display is available
- **"capture session not initialized"**: Internal error, check logs
- **"failed to get frame"**: May indicate permission revoked or display disconnected

## Development Notes

### Adding New Features

- **Multi-display support**: Use `GetDisplayIDs()` to enumerate displays
- **Window capture**: Modify `SCContentFilter` to capture specific windows
- **Hardware encoding**: Consider VideoToolbox for H.264 encoding

### Debugging

Enable verbose logging to see capture lifecycle:

```go
log.SetLevel(log.DebugLevel)
```

Check system logs for ScreenCaptureKit errors:

```bash
log show --predicate 'subsystem == "com.apple.ScreenCaptureKit"' --last 1h
```

## Files

- `screencapture_macos.m`: Objective-C implementation using ScreenCaptureKit
- `screencapture_macos.h`: C interface header
- `screencapture_macos_cgo.go`: Go CGO bridge and frame processing
- `screencapture_darwin.go`: Darwin-specific Go implementation
- `screencapture_darwin_test.go`: Unit tests

