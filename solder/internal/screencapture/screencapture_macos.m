//go:build darwin
// +build darwin

#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreVideo/CoreVideo.h>
#import <CoreGraphics/CoreGraphics.h>
#import "screencapture_macos.h"

// Frame callback function pointer type
typedef void (*FrameCallback)(void* buffer, size_t size, uint32_t width, uint32_t height, int64_t timestamp);

// Static cache for SCDisplay objects to avoid blocking semaphores in CGO
static NSMutableDictionary<NSNumber*, SCDisplay*>* g_displayCache = nil;
static dispatch_once_t g_cacheOnceToken = 0;
static dispatch_queue_t g_cacheQueue = nil;

// Capture session wrapper
@interface ScreenCaptureSession : NSObject <SCStreamOutput, SCStreamDelegate>

@property (nonatomic, assign) SCStream* stream;
@property (nonatomic, assign) SCContentFilter* contentFilter;
@property (nonatomic, assign) SCStreamConfiguration* streamConfig;
@property (nonatomic, assign) FrameCallback frameCallback;
@property (nonatomic, assign) BOOL isCapturing;
@property (nonatomic, assign) uint32_t displayID;
@property (nonatomic, assign) int maxWidth;
@property (nonatomic, assign) int maxHeight;
@property (nonatomic, strong) dispatch_queue_t captureQueue;

@end

@implementation ScreenCaptureSession

+ (void)initializeDisplayCache {
    dispatch_once(&g_cacheOnceToken, ^{
        g_displayCache = [[NSMutableDictionary alloc] init];
        g_cacheQueue = dispatch_queue_create("com.borg.screencapture.cache", DISPATCH_QUEUE_SERIAL);
        
        // Pre-fetch displays synchronously with timeout (only once during initialization)
        // This happens before CGO calls, so blocking is acceptable here
        if (@available(macOS 12.3, *)) {
            __block BOOL fetchComplete = NO;
            dispatch_semaphore_t semaphore = dispatch_semaphore_create(0);
            
            [SCShareableContent getShareableContentExcludingDesktopWindows:YES
                                                        onScreenWindowsOnly:YES
                                                         completionHandler:^(SCShareableContent* content, NSError* error) {
                if (content && !error) {
                    dispatch_async(g_cacheQueue, ^{
                        for (SCDisplay* display in content.displays) {
                            NSNumber* key = @(display.displayID);
                            g_displayCache[key] = display;
                        }
                        NSLog(@"Cached %lu displays", (unsigned long)g_displayCache.count);
                        fetchComplete = YES;
                        dispatch_semaphore_signal(semaphore);
                    });
                } else {
                    if (error) {
                        NSLog(@"Failed to prefetch displays: %@", error);
                    }
                    fetchComplete = YES;
                    dispatch_semaphore_signal(semaphore);
                }
            }];
            
            // Wait with timeout (1 second max) for initial cache population
            // This only happens once during initialization, before CGO calls
            dispatch_time_t timeout = dispatch_time(DISPATCH_TIME_NOW, 1 * NSEC_PER_SEC);
            dispatch_semaphore_wait(semaphore, timeout);
        }
    });
}

- (instancetype)init {
    self = [super init];
    if (self) {
        _isCapturing = NO;
        _captureQueue = dispatch_queue_create("com.borg.screencapture", DISPATCH_QUEUE_SERIAL);
        // Ensure cache is initialized
        [[self class] initializeDisplayCache];
    }
    return self;
}

// Helper function to get SCDisplay from CGDirectDisplayID (non-blocking, uses cache)
+ (SCDisplay*)displayForDisplayID:(CGDirectDisplayID)displayID {
    // Ensure cache is initialized (this happens once, before CGO calls)
    [self initializeDisplayCache];
    
    if (@available(macOS 12.3, *)) {
        __block SCDisplay* result = nil;
        
        // Check cache only (synchronous, non-blocking cache lookup)
        // Cache should be populated during initialization
        dispatch_sync(g_cacheQueue, ^{
            NSNumber* key = @(displayID);
            result = g_displayCache[key];
        });
        
        return result;
    }
    return nil;
}

- (BOOL)startCaptureWithDisplayID:(CGDirectDisplayID)displayID
                          maxWidth:(int)maxWidth
                         maxHeight:(int)maxHeight {
    if (self.isCapturing) {
        return NO;
    }
    
    self.displayID = displayID;
    self.maxWidth = maxWidth;
    self.maxHeight = maxHeight;
    
    // Create content filter for display
    // Get SCDisplay object from CGDirectDisplayID
    SCDisplay* display = [[self class] displayForDisplayID:displayID];
    if (!display) {
        NSLog(@"Failed to get SCDisplay for displayID: %u", displayID);
        return NO;
    }
    
    SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplay:display
                                                      excludingWindows:@[]];
    if (!filter) {
        return NO;
    }
    self.contentFilter = filter;
    
    // Create stream configuration
    SCStreamConfiguration* config = [[SCStreamConfiguration alloc] init];
    config.width = maxWidth > 0 ? maxWidth : (int)CGDisplayPixelsWide(displayID);
    config.height = maxHeight > 0 ? maxHeight : (int)CGDisplayPixelsHigh(displayID);
    config.pixelFormat = kCVPixelFormatType_32BGRA;
    config.showsCursor = YES;
    config.queueDepth = 3;
    config.minimumFrameInterval = CMTimeMake(1, 30); // 30 FPS max
    
    self.streamConfig = config;
    
    // Create stream
    SCStream* stream = [[SCStream alloc] initWithFilter:filter
                                           configuration:config
                                                delegate:self];
    if (!stream) {
        return NO;
    }
    self.stream = stream;
    
    // Add stream output
    NSError* error = nil;
    BOOL success = [stream addStreamOutput:self
                                      type:SCStreamOutputTypeScreen
                                sampleHandlerQueue:self.captureQueue
                                         error:&error];
    if (!success) {
        NSLog(@"Failed to add stream output: %@", error);
        return NO;
    }
    
    // Start capture
    // Note: startCaptureWithCompletionHandler returns void, not BOOL
    // Success/failure is handled in the completion handler
    [stream startCaptureWithCompletionHandler:^(NSError* _Nullable error) {
        if (error) {
            NSLog(@"Failed to start capture: %@", error);
            self.isCapturing = NO;
        } else {
            self.isCapturing = YES;
        }
    }];
    
    // Return YES optimistically - actual success is handled asynchronously in completion handler
    return YES;
}

- (void)stopCapture {
    if (!self.isCapturing || !self.stream) {
        return;
    }
    
    [self.stream stopCaptureWithCompletionHandler:^(NSError* _Nullable error) {
        if (error) {
            NSLog(@"Error stopping capture: %@", error);
        }
        self.isCapturing = NO;
    }];
    
    [self.stream removeStreamOutput:self type:SCStreamOutputTypeScreen error:nil];
    self.stream = nil;
    self.contentFilter = nil;
    self.streamConfig = nil;
}

- (void)stream:(SCStream*)stream didOutputSampleBuffer:(CMSampleBufferRef)sampleBuffer
          ofType:(SCStreamOutputType)type {
    if (type != SCStreamOutputTypeScreen || !self.frameCallback) {
        return;
    }
    
    CVImageBufferRef imageBuffer = CMSampleBufferGetImageBuffer(sampleBuffer);
    if (!imageBuffer) {
        return;
    }
    
    CVPixelBufferLockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
    
    size_t width = CVPixelBufferGetWidth(imageBuffer);
    size_t height = CVPixelBufferGetHeight(imageBuffer);
    size_t bytesPerRow = CVPixelBufferGetBytesPerRow(imageBuffer);
    
    void* baseAddress = CVPixelBufferGetBaseAddress(imageBuffer);
    if (!baseAddress) {
        CVPixelBufferUnlockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
        return;
    }
    
    // Calculate actual data size (accounting for padding)
    size_t dataSize = height * bytesPerRow;
    
    // Allocate buffer and copy data
    void* buffer = malloc(dataSize);
    if (buffer) {
        memcpy(buffer, baseAddress, dataSize);
        
        // Get timestamp
        CMTime timestamp = CMSampleBufferGetPresentationTimeStamp(sampleBuffer);
        int64_t timestampNs = CMTimeGetSeconds(timestamp) * 1000000000;
        
        // Call callback
        self.frameCallback(buffer, dataSize, (uint32_t)width, (uint32_t)height, timestampNs);
    }
    
    CVPixelBufferUnlockBaseAddress(imageBuffer, kCVPixelBufferLock_ReadOnly);
}

- (void)stream:(SCStream*)stream didStopWithError:(NSError*)error {
    if (error) {
        NSLog(@"Stream stopped with error: %@", error);
    }
    self.isCapturing = NO;
}

@end

// C interface functions

static ScreenCaptureSession* g_session = nil;

void* CreateCaptureSession(void) {
    if (g_session) {
        return (__bridge void*)g_session;
    }
    g_session = [[ScreenCaptureSession alloc] init];
    return (__bridge void*)g_session;
}

void DestroyCaptureSession(void* session) {
    if (session) {
        ScreenCaptureSession* s = (__bridge ScreenCaptureSession*)session;
        [s stopCapture];
        if (s == g_session) {
            g_session = nil;
        }
    }
}

int StartCapture(void* session, uint32_t displayID, int maxWidth, int maxHeight) {
    if (!session) {
        return 0;
    }
    ScreenCaptureSession* s = (__bridge ScreenCaptureSession*)session;
    return [s startCaptureWithDisplayID:displayID maxWidth:maxWidth maxHeight:maxHeight] ? 1 : 0;
}

void StopCapture(void* session) {
    if (session) {
        ScreenCaptureSession* s = (__bridge ScreenCaptureSession*)session;
        [s stopCapture];
    }
}

int IsCapturing(void* session) {
    if (!session) {
        return 0;
    }
    ScreenCaptureSession* s = (__bridge ScreenCaptureSession*)session;
    return s.isCapturing ? 1 : 0;
}

void SetFrameCallback(void* session, FrameCallback callback) {
    if (session) {
        ScreenCaptureSession* s = (__bridge ScreenCaptureSession*)session;
        // Use provided callback or default to bridge function
        if (callback) {
            s.frameCallback = callback;
        } else {
            // Default to bridge function exported from Go
            s.frameCallback = frameCallbackBridge;
        }
    }
}

int HasScreenRecordingPermission(void) {
    // Simplified permission check that doesn't use blocking async APIs
    // The actual permission will be validated when capture is attempted
    // This avoids crashing CGO calls with blocking semaphores
    if (@available(macOS 12.3, *)) {
        // Check if ScreenCaptureKit framework is available
        // and if we can access display information (basic availability check)
        CGDirectDisplayID displayID = CGMainDisplayID();
        if (displayID == 0) {
            return 0; // No displays available
        }
        // Check if we can get display dimensions (lightweight synchronous check)
        uint32_t width = CGDisplayPixelsWide(displayID);
        uint32_t height = CGDisplayPixelsHigh(displayID);
        if (width == 0 || height == 0) {
            return 0; // Display info not accessible
        }
        // Framework is available and displays are accessible
        // Note: Actual ScreenCaptureKit permission validation happens during capture attempt
        // This avoids blocking CGO calls with semaphores from SCShareableContent API
        return 1;
    }
    return 0; // macOS version too old
}

void RequestScreenRecordingPermission(void) {
    // On macOS, permission is requested automatically when trying to capture
    // We can't programmatically request it from CGO context
    // The permission dialog will appear automatically when capture is first attempted
    // This function is a no-op to avoid blocking CGO calls
    // Actual permission request happens during capture attempt in startCaptureWithDisplayID:
    if (@available(macOS 12.3, *)) {
        // Permission request will happen automatically when capture is attempted
        // No action needed here to avoid blocking CGO calls
    }
}

uint32_t GetPrimaryDisplayID(void) {
    return CGMainDisplayID();
}

int GetDisplayCount(void) {
    uint32_t count = 0;
    CGGetActiveDisplayList(0, NULL, &count);
    return (int)count;
}

void GetDisplayIDs(uint32_t* displays, int count) {
    if (!displays || count <= 0) {
        return;
    }
    uint32_t displayCount = (uint32_t)count;
    CGGetActiveDisplayList(displayCount, displays, &displayCount);
}

