// One aggregate device holds the tap as input and the headset as output, so a
// single IOProc gives both sides of the same time slice. No ring buffer, no
// drift between two clocks. That is where the latency saving is.
//
// ObjC because CATapDescription is an ObjC class.

#import <Foundation/Foundation.h>
#import <CoreAudio/CoreAudio.h>
#import <CoreAudio/CATapDescription.h>
#import <CoreAudio/AudioHardwareTapping.h>

// Implemented in Rust, called on the render thread.
typedef void (*occam_render_fn)(const float *in, uint32_t in_channels,
                                float *out, uint32_t out_channels,
                                uint32_t frames, void *ctx);

void occam_live_stop(void);

static AudioObjectID gTap = kAudioObjectUnknown;
static AudioObjectID gAggregate = kAudioObjectUnknown;
static AudioDeviceIOProcID gProc = NULL;
static occam_render_fn gRender = NULL;
static void *gCtx = NULL;

static OSStatus occam_ioproc(AudioObjectID device,
                             const AudioTimeStamp *now,
                             const AudioBufferList *inputData,
                             const AudioTimeStamp *inputTime,
                             AudioBufferList *outputData,
                             const AudioTimeStamp *outputTime,
                             void *client) {
    (void)device; (void)now; (void)inputTime; (void)outputTime; (void)client;

    if (!gRender || !outputData || outputData->mNumberBuffers == 0) return noErr;

    AudioBuffer *out = &outputData->mBuffers[0];
    uint32_t outCh = out->mNumberChannels;
    uint32_t frames = outCh ? out->mDataByteSize / (sizeof(float) * outCh) : 0;
    if (frames == 0) return noErr;

    const float *in = NULL;
    uint32_t inCh = 0;
    if (inputData && inputData->mNumberBuffers > 0) {
        in = (const float *)inputData->mBuffers[0].mData;
        inCh = inputData->mBuffers[0].mNumberChannels;
    }

    gRender(in, inCh, (float *)out->mData, outCh, frames, gCtx);
    return noErr;
}

static AudioObjectID occam_find_output(const char *needle);

static CFStringRef copy_device_uid(AudioObjectID device) {
    AudioObjectPropertyAddress addr = {
        kAudioDevicePropertyDeviceUID,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    CFStringRef uid = NULL;
    UInt32 size = sizeof(uid);
    if (AudioObjectGetPropertyData(device, &addr, 0, NULL, &size, &uid) != noErr) return NULL;
    return uid;
}

static AudioObjectID occam_find_output(const char *needle) {
    AudioObjectPropertyAddress addr = {
        kAudioHardwarePropertyDevices,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    UInt32 size = 0;
    if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &addr, 0, NULL, &size) != noErr) {
        return kAudioObjectUnknown;
    }
    UInt32 count = size / sizeof(AudioObjectID);
    AudioObjectID *ids = malloc(size);
    if (!ids) return kAudioObjectUnknown;
    if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &addr, 0, NULL, &size, ids) != noErr) {
        free(ids);
        return kAudioObjectUnknown;
    }

    NSString *want = [NSString stringWithUTF8String:needle];
    AudioObjectID found = kAudioObjectUnknown;

    for (UInt32 i = 0; i < count && found == kAudioObjectUnknown; i++) {
        // Must have output streams, or it is an input-only device.
        AudioObjectPropertyAddress streams = {
            kAudioDevicePropertyStreams,
            kAudioObjectPropertyScopeOutput,
            kAudioObjectPropertyElementMain,
        };
        UInt32 streamSize = 0;
        if (AudioObjectGetPropertyDataSize(ids[i], &streams, 0, NULL, &streamSize) != noErr ||
            streamSize == 0) {
            continue;
        }

        AudioObjectPropertyAddress nameAddr = {
            kAudioObjectPropertyName,
            kAudioObjectPropertyScopeGlobal,
            kAudioObjectPropertyElementMain,
        };
        CFStringRef name = NULL;
        UInt32 nameSize = sizeof(name);
        if (AudioObjectGetPropertyData(ids[i], &nameAddr, 0, NULL, &nameSize, &name) != noErr) {
            continue;
        }
        if ([(__bridge NSString *)name rangeOfString:want].location != NSNotFound) {
            found = ids[i];
        }
        CFRelease(name);
    }
    free(ids);
    return found;
}

double occam_device_rate(AudioObjectID device) {
    AudioObjectPropertyAddress addr = {
        kAudioDevicePropertyNominalSampleRate,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    Float64 rate = 0;
    UInt32 size = sizeof(rate);
    if (AudioObjectGetPropertyData(device, &addr, 0, NULL, &size, &rate) != noErr) return 0;
    return (double)rate;
}

// The device may refuse and pick its own, so read it back rather than assume.
void occam_set_buffer_frames(AudioObjectID device, uint32_t frames) {
    AudioObjectPropertyAddress addr = {
        kAudioDevicePropertyBufferFrameSize,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    AudioObjectSetPropertyData(device, &addr, 0, NULL, sizeof(frames), &frames);
}

uint32_t occam_buffer_frames(AudioObjectID device) {
    AudioObjectPropertyAddress addr = {
        kAudioDevicePropertyBufferFrameSize,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    UInt32 frames = 0;
    UInt32 size = sizeof(frames);
    if (AudioObjectGetPropertyData(device, &addr, 0, NULL, &size, &frames) != noErr) return 0;
    return frames;
}

// Fills counts[0..n) with the channels in each buffer of a scope's stream
// configuration, and returns how many buffers there are. An aggregate can
// present several buffers rather than one interleaved block, which is exactly
// the assumption that broke the first run.
uint32_t occam_describe(AudioObjectID device, int input, uint32_t *counts, uint32_t max) {
    AudioObjectPropertyAddress addr = {
        kAudioDevicePropertyStreamConfiguration,
        input ? kAudioObjectPropertyScopeInput : kAudioObjectPropertyScopeOutput,
        kAudioObjectPropertyElementMain,
    };
    UInt32 size = 0;
    if (AudioObjectGetPropertyDataSize(device, &addr, 0, NULL, &size) != noErr || size == 0) {
        return 0;
    }
    AudioBufferList *list = malloc(size);
    if (!list) return 0;
    if (AudioObjectGetPropertyData(device, &addr, 0, NULL, &size, list) != noErr) {
        free(list);
        return 0;
    }
    uint32_t n = list->mNumberBuffers;
    for (uint32_t i = 0; i < n && i < max; i++) counts[i] = list->mBuffers[i].mNumberChannels;
    free(list);
    return n;
}

// Names every output device, so an ambiguous substring is visible rather than
// silently resolving to the wrong endpoint.
uint32_t occam_list_outputs(char *out, uint32_t cap) {
    AudioObjectPropertyAddress addr = {
        kAudioHardwarePropertyDevices,
        kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    UInt32 size = 0;
    if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &addr, 0, NULL, &size) != noErr) {
        return 0;
    }
    UInt32 count = size / sizeof(AudioObjectID);
    AudioObjectID *ids = malloc(size);
    if (!ids) return 0;
    if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &addr, 0, NULL, &size, ids) != noErr) {
        free(ids);
        return 0;
    }

    NSMutableString *acc = [NSMutableString string];
    for (UInt32 i = 0; i < count; i++) {
        AudioObjectPropertyAddress streams = {
            kAudioDevicePropertyStreams, kAudioObjectPropertyScopeOutput,
            kAudioObjectPropertyElementMain,
        };
        UInt32 streamSize = 0;
        if (AudioObjectGetPropertyDataSize(ids[i], &streams, 0, NULL, &streamSize) != noErr ||
            streamSize == 0) continue;

        AudioObjectPropertyAddress nameAddr = {
            kAudioObjectPropertyName, kAudioObjectPropertyScopeGlobal,
            kAudioObjectPropertyElementMain,
        };
        CFStringRef name = NULL;
        UInt32 nameSize = sizeof(name);
        if (AudioObjectGetPropertyData(ids[i], &nameAddr, 0, NULL, &nameSize, &name) != noErr) continue;
        [acc appendFormat:@"%@\n", (__bridge NSString *)name];
        CFRelease(name);
    }
    free(ids);

    const char *utf8 = [acc UTF8String];
    uint32_t len = (uint32_t)strlen(utf8);
    if (len >= cap) len = cap - 1;
    memcpy(out, utf8, len);
    out[len] = 0;
    return len;
}

// Exact match first, substring only as a fallback: this device publishes both
// a Game and a Chat endpoint and "BlackShark" hit Chat.
AudioObjectID occam_find_output_exact(const char *wanted) {
    AudioObjectID exact = kAudioObjectUnknown;
    AudioObjectPropertyAddress addr = {
        kAudioHardwarePropertyDevices, kAudioObjectPropertyScopeGlobal,
        kAudioObjectPropertyElementMain,
    };
    UInt32 size = 0;
    if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &addr, 0, NULL, &size) != noErr) {
        return kAudioObjectUnknown;
    }
    UInt32 count = size / sizeof(AudioObjectID);
    AudioObjectID *ids = malloc(size);
    if (!ids) return kAudioObjectUnknown;
    if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &addr, 0, NULL, &size, ids) != noErr) {
        free(ids);
        return kAudioObjectUnknown;
    }

    NSString *want = [NSString stringWithUTF8String:wanted];
    for (UInt32 i = 0; i < count && exact == kAudioObjectUnknown; i++) {
        AudioObjectPropertyAddress streams = {
            kAudioDevicePropertyStreams, kAudioObjectPropertyScopeOutput,
            kAudioObjectPropertyElementMain,
        };
        UInt32 streamSize = 0;
        if (AudioObjectGetPropertyDataSize(ids[i], &streams, 0, NULL, &streamSize) != noErr ||
            streamSize == 0) continue;

        AudioObjectPropertyAddress nameAddr = {
            kAudioObjectPropertyName, kAudioObjectPropertyScopeGlobal,
            kAudioObjectPropertyElementMain,
        };
        CFStringRef name = NULL;
        UInt32 nameSize = sizeof(name);
        if (AudioObjectGetPropertyData(ids[i], &nameAddr, 0, NULL, &nameSize, &name) != noErr) continue;
        if ([(__bridge NSString *)name isEqualToString:want]) exact = ids[i];
        CFRelease(name);
    }
    free(ids);
    return exact != kAudioObjectUnknown ? exact : occam_find_output(wanted);
}

OSStatus occam_live_start(AudioObjectID output, uint32_t wantFrames,
                          occam_render_fn render, void *ctx) {
    if (@available(macOS 14.2, *)) {
        gRender = render;
        gCtx = ctx;

        // Excluding ourselves is load-bearing. The tap mutes what it captures,
        // and our own rendered output goes to the same device, so a tap that
        // excludes nothing captures and then mutes us as well.
        AudioObjectID self = kAudioObjectUnknown;
        pid_t pid = getpid();
        AudioObjectPropertyAddress translate = {
            kAudioHardwarePropertyTranslatePIDToProcessObject,
            kAudioObjectPropertyScopeGlobal,
            kAudioObjectPropertyElementMain,
        };
        UInt32 selfSize = sizeof(self);
        AudioObjectGetPropertyData(kAudioObjectSystemObject, &translate,
                                   sizeof(pid), &pid, &selfSize, &self);

        NSArray *exclude = (self != kAudioObjectUnknown) ? @[@(self)] : @[];
        CATapDescription *desc =
            [[CATapDescription alloc] initStereoGlobalTapButExcludeProcesses:exclude];
        desc.name = @"occam";
        desc.privateTap = YES;
        desc.muteBehavior = CATapMuted;

        OSStatus s = AudioHardwareCreateProcessTap(desc, &gTap);
        if (s != noErr) return s;

        CFStringRef tapUID = (__bridge_retained CFStringRef)[desc.UUID UUIDString];
        CFStringRef outUID = copy_device_uid(output);
        if (!outUID) {
            AudioHardwareDestroyProcessTap(gTap);
            gTap = kAudioObjectUnknown;
            return kAudioHardwareBadObjectError;
        }

        NSDictionary *aggregate = @{
            @(kAudioAggregateDeviceNameKey): @"occam live",
            @(kAudioAggregateDeviceUIDKey): @"com.dappermint.occam.live",
            @(kAudioAggregateDeviceMainSubDeviceKey): (__bridge NSString *)outUID,
            @(kAudioAggregateDeviceIsPrivateKey): @YES,
            @(kAudioAggregateDeviceIsStackedKey): @NO,
            @(kAudioAggregateDeviceSubDeviceListKey): @[
                @{ @(kAudioSubDeviceUIDKey): (__bridge NSString *)outUID },
            ],
            @(kAudioAggregateDeviceTapListKey): @[
                @{
                    @(kAudioSubTapUIDKey): (__bridge NSString *)tapUID,
                    @(kAudioSubTapDriftCompensationKey): @YES,
                },
            ],
        };

        s = AudioHardwareCreateAggregateDevice((__bridge CFDictionaryRef)aggregate, &gAggregate);
        CFRelease(outUID);
        CFRelease(tapUID);
        if (s != noErr) {
            AudioHardwareDestroyProcessTap(gTap);
            gTap = kAudioObjectUnknown;
            return s;
        }

        if (wantFrames) occam_set_buffer_frames(gAggregate, wantFrames);

        s = AudioDeviceCreateIOProcID(gAggregate, occam_ioproc, NULL, &gProc);
        if (s != noErr) {
            occam_live_stop();
            return s;
        }
        s = AudioDeviceStart(gAggregate, gProc);
        if (s != noErr) {
            occam_live_stop();
            return s;
        }
        return noErr;
    }
    return kAudioHardwareUnsupportedOperationError;
}

AudioObjectID occam_aggregate_id(void) { return gAggregate; }

void occam_live_stop(void) {
    if (gAggregate != kAudioObjectUnknown && gProc) {
        AudioDeviceStop(gAggregate, gProc);
        AudioDeviceDestroyIOProcID(gAggregate, gProc);
        gProc = NULL;
    }
    if (gAggregate != kAudioObjectUnknown) {
        AudioHardwareDestroyAggregateDevice(gAggregate);
        gAggregate = kAudioObjectUnknown;
    }
    if (gTap != kAudioObjectUnknown) {
        if (@available(macOS 14.2, *)) {
            AudioHardwareDestroyProcessTap(gTap);
        }
        gTap = kAudioObjectUnknown;
    }
    gRender = NULL;
    gCtx = NULL;
}
