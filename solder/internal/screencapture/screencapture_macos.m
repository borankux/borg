#import <Foundation/Foundation.h>
#import <ScreenCaptureKit/ScreenCaptureKit.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreVideo/CoreVideo.h>
#import <CoreGraphics/CoreGraphics.h>
#import "screencapture_macos.h"

// Frame callback function pointer type
typedef void (*FrameCallback)(void* buffer, size_t size, uint32_t width, uint32_t height, int64_t timestamp);

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

- (instancetype)init {
    self = [super init];
    if (self) {
        _isCapturing = NO;
        _captureQueue = dispatch_queue_create("com.borg.screencapture", DISPATCH_QUEUE_SERIAL);
    }
    return self;
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
    SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplayID:displayID
                                                         excludingWindows:nil];
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
    success = [stream startCaptureWithCompletionHandler:^(NSError* _Nullable error) {
        if (error) {
            NSLog(@"Failed to start capture: %@", error);
            self.isCapturing = NO;
        } else {
            self.isCapturing = YES;
        }
    }];
    
    return success;
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
    if (@available(macOS 12.3, *)) {
        // Check if we can create a content filter (requires permission)
        CGDirectDisplayID displayID = CGMainDisplayID();
        SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplayID:displayID
                                                             excludingWindows:nil];
        return filter != nil ? 1 : 0;
    }
    return 0;
}

void RequestScreenRecordingPermission(void) {
    // On macOS, permission is requested automatically when trying to capture
    // We can't programmatically request it, but we can try to create a filter
    // which will trigger the system permission dialog
    if (@available(macOS 12.3, *)) {
        CGDirectDisplayID displayID = CGMainDisplayID();
        SCContentFilter* filter = [[SCContentFilter alloc] initWithDisplayID:displayID
                                                             excludingWindows:nil];
        // The act of creating the filter may trigger permission request
        (void)filter;
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

