# macOS Screen Recording Permission Guide

## How macOS Permission System Works

macOS **does not allow programmatic permission requests** for Screen Recording. The system automatically shows a permission dialog when your app first attempts to capture the screen. However, this dialog only appears **once** - if the user denies it, you must manually grant permission in System Settings.

## Current Implementation Behavior

### 1. **Automatic Permission Check**
The agent checks for permission on startup:
- If permission is **granted**: Screen capture is enabled
- If permission is **not granted**: Screen capture is disabled, agent continues running normally

### 2. **Permission Request Flow**
When you try to capture:
- First attempt: macOS shows permission dialog automatically
- If granted: Capture works immediately
- If denied: Must manually grant in System Settings

## How to Grant Permission

### Method 1: Automatic (First Time)
1. Run the agent: `./solder --config config.yaml`
2. When screen capture is attempted, macOS will show a dialog
3. Click **"Open System Settings"** or **"OK"**
4. Enable the checkbox for your terminal/application

### Method 2: Manual (If Denied or Not Prompted)

#### Option A: Via System Settings UI
1. Open **System Settings** (or **System Preferences** on macOS 12)
2. Go to **Privacy & Security** → **Screen Recording**
3. Find your terminal application (Terminal, iTerm, etc.) or the `solder` binary
4. Enable the checkbox

#### Option B: Via Command Line (macOS 13+)
```bash
# Open Screen Recording settings directly
open "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
```

#### Option C: Grant Permission to Terminal
If running from Terminal/iTerm, grant permission to the terminal app:
```bash
# For Terminal.app
sudo sqlite3 /Library/Application\ Support/com.apple.TCC/TCC.db "INSERT OR REPLACE INTO access VALUES('kTCCServiceScreenCapture','com.apple.Terminal',1,2,4,1,NULL,NULL,NULL,'UNUSED',NULL,0,1541440109);"

# Note: This requires admin privileges and may not work on newer macOS versions
```

**Better approach**: Grant permission to Terminal/iTerm via System Settings UI.

## Improving Permission Handling

### Enhanced Error Messages

The current implementation logs a message. You can improve it by:

1. **More descriptive startup message**:
```go
if !screenCapture.IsEnabled() {
    log.Printf(`
⚠️  Screen Recording permission not granted.

To enable screen monitoring:
1. Open System Settings > Privacy & Security > Screen Recording
2. Enable the checkbox for: %s
3. Restart the agent

Screen monitoring will be disabled until permission is granted.
`, os.Args[0])
}
```

2. **Periodic permission re-check** (optional):
```go
// Check permission every 30 seconds and log if it becomes available
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if hasScreenRecordingPermission() && !screenCapture.IsEnabled() {
                log.Println("Screen Recording permission granted! Restart agent to enable screen monitoring.")
            }
        }
    }
}()
```

### Helper Script

Create a helper script to check and guide users:

```bash
#!/bin/bash
# check-permission.sh

echo "Checking Screen Recording permission..."

# Try to create a test filter (requires permission)
# This will trigger permission dialog if not granted

if ./solder --check-permission 2>&1 | grep -q "permission"; then
    echo ""
    echo "❌ Permission not granted"
    echo ""
    echo "To grant permission:"
    echo "1. Open System Settings"
    echo "2. Go to Privacy & Security > Screen Recording"
    echo "3. Enable checkbox for: $(basename $0)"
    echo ""
    echo "Or run: open 'x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture'"
else
    echo "✅ Permission granted"
fi
```

## Troubleshooting

### Permission Granted But Still Fails

1. **Restart the agent** after granting permission
2. **Check which app needs permission**:
   - If running from Terminal: Grant to Terminal.app
   - If running as binary: Grant to the binary itself
   - If running from IDE: Grant to the IDE (Xcode, VS Code, etc.)

### Permission Dialog Not Appearing

1. **Check if already denied**: Go to System Settings and check if your app is listed (even if unchecked)
2. **Reset TCC database** (advanced, requires admin):
   ```bash
   sudo tccutil reset ScreenCapture
   ```
   Then restart and try again

### Permission Works But Capture Fails

1. **Check macOS version**: Requires macOS 12.3+
2. **Check logs**: Look for ScreenCaptureKit errors
3. **Verify display**: Ensure at least one display is connected

## Best Practices

1. **Check permission on startup**: ✅ Already implemented
2. **Graceful degradation**: ✅ Agent continues without screen capture
3. **Clear error messages**: ✅ Logs permission status
4. **User guidance**: Provide instructions in logs/docs
5. **Don't block startup**: ✅ Permission check doesn't fail agent startup

## Code Example: Enhanced Permission Handling

```go
func checkAndLogPermission() {
    if runtime.GOOS != "darwin" {
        return
    }
    
    if hasScreenRecordingPermission() {
        log.Println("✅ Screen Recording permission granted")
    } else {
        log.Printf(`
⚠️  Screen Recording Permission Required

Screen monitoring is disabled because Screen Recording permission is not granted.

To enable:
1. Open System Settings > Privacy & Security > Screen Recording
2. Enable checkbox for: %s
3. Restart the agent

The agent will continue running without screen monitoring.
`, os.Args[0])
    }
}
```

## Testing Permission

You can test permission status:

```bash
# Build the agent
go build ./cmd/agent

# Run with screen capture enabled in config
./agent --config config.yaml

# Check logs for permission status
# Look for: "Screen Recording permission not granted" or "Screen capture enabled"
```

## Summary

- ✅ **Automatic**: Permission dialog appears on first capture attempt
- ✅ **Manual**: Grant via System Settings if denied
- ✅ **Graceful**: Agent continues without screen capture if permission not granted
- ✅ **Check**: Permission is checked on startup
- ⚠️ **Limitation**: Cannot programmatically request permission (macOS restriction)

