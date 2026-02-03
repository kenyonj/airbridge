# Virtual AirPlay Receiver Investigation

## Issue #5: Virtual AirPlay devices (AirPlay receiver/bridge)

This document contains the research and findings for implementing virtual AirPlay receivers in Airbridge.

## Executive Summary

After investigating Go-based RAOP/AirPlay receiver implementations, I found that while **technically possible**, a full implementation would require significant effort due to:

1. **Library maturity issues** - Available Go libraries have incomplete APIs or build problems
2. **Audio pipeline complexity** - Need to handle decryption, decompression (ALAC), format conversion
3. **Protocol complexity** - RAOP/AirPlay 1 requires RTSP setup, RTP streaming, and timing sync
4. **AirPlay 2 complications** - Requires HomeKit pairing, encryption, and multi-room sync

## Available Go Libraries

### 1. github.com/maghul/go.raopd
- **Status**: Unmaintained (last update 2016)
- **Protocol**: AirPlay 1 / RAOP
- **Pros**:
  - Pure Go implementation
  - Handles RTSP, RTP, encryption, ALAC decompression
  - Provides Sink interface for implementing receivers
- **Cons**:
  - Build errors in dependencies (go.slf has unused import)
  - Incomplete API (AudioWriter methods commented out)
  - Requires multiple dependencies (go.dnssd, go.alac, go.dbus)
  - No active maintenance

### 2. github.com/openairplay/goplay2
- **Status**: Active (AirPlay 2 focus)
- **Protocol**: AirPlay 2 only
- **Pros**:
  - Modern AirPlay 2 support
  - Multi-room sync
  - HomeKit integration
- **Cons**:
  - Does NOT support AirPlay 1
  - Requires external dependencies (PortAudio, PulseAudio, libfdk-aac)
  - Does not run on Windows
  - More complex to integrate

## Architecture Design

If we were to implement this feature, here's the recommended architecture:

```
┌─────────────────┐
│ AirPlay Client  │  (iPhone, Mac, etc.)
└────────┬────────┘
         │ AirPlay/RAOP Protocol
         ▼
┌─────────────────┐
│ RAOP Receiver   │  (Virtual AirPlay device)
│   - RTSP Server │
│   - RTP Handler │
│   - ALAC Decoder│
│   - AES Decrypt │
└────────┬────────┘
         │ PCM Audio Stream (44.1kHz, 16-bit, stereo)
         ▼
┌─────────────────┐
│ Audio Forwarder │
└────────┬────────┘
         ├──────────────────┬──────────────────┐
         ▼                  ▼                  ▼
  ┌──────────┐      ┌──────────┐      ┌──────────┐
  │ AirPlay  │      │Chromecast│      │  DLNA    │
  │  Output  │      │  Output  │      │ Output   │
  └──────────┘      └──────────┘      └──────────┘
```

## Technical Challenges

### 1. Audio Stream Handling
- **Input**: Encrypted, ALAC-compressed audio over RTP
- **Processing**: Decrypt → Decompress → Convert to PCM
- **Output**: Need to re-encode for target protocol (if not PCM)

### 2. Timing and Synchronization
- RAOP uses RTP timestamps for precise audio timing
- Need to buffer appropriately to prevent gaps/glitches
- Multi-room sync would require careful clock management

### 3. Format Conversion
- RAOP delivers 16-bit PCM at 44.1kHz stereo
- May need to transcode for different target devices
- FFmpeg would likely be required (adds latency)

### 4. Connection Management
- Need to handle connection/disconnection gracefully
- Support multiple simultaneous receivers
- Handle errors in downstream players

## Proposed Implementation Path

### Phase 1: Research & Proof of Concept (Current)
- [x] Research available libraries
- [x] Evaluate technical feasibility
- [x] Document architecture
- [ ] Create minimal working prototype with shairport-sync wrapper

### Phase 2: Wrapper Implementation (Alternative Approach)
Instead of using go.raopd, use **shairport-sync** as a subprocess:
- shairport-sync is mature and battle-tested
- Can pipe audio output to stdin of another process
- Already handles all RAOP complexity
- Example: `shairport-sync --name "Bridge" --output pipe -- /path/to/handler`

Benefits:
- Proven implementation
- Active maintenance
- Handles AirPlay 1 and 2
- Less complexity in Go code

Drawbacks:
- External dependency
- Requires installation of shairport-sync
- Subprocess management overhead

### Phase 3: Native Go Implementation
- Fix or fork go.raopd to resolve build issues
- Implement complete Sink interface with audio streaming
- Add audio forwarding to existing players
- Handle encryption and ALAC decompression
- Add web UI for managing virtual receivers

## Code Example: Wrapper Approach

```go
// Pseudocode for shairport-sync wrapper
type ShairportReceiver struct {
    name    string
    cmd     *exec.Cmd
    output  OutputPlayer
}

func (r *ShairportReceiver) Start() error {
    // Start shairport-sync in pipe mode
    r.cmd = exec.Command("shairport-sync",
        "--name", r.name,
        "--output", "stdout",
        "--",
    )
    
    stdout, _ := r.cmd.StdoutPipe()
    r.cmd.Start()
    
    // Forward PCM audio to output player
    go r.output.PlayStream(ctx, stdout, 80)
    
    return nil
}
```

## Use Cases Enabled

Once implemented, this would enable:

1. **AirPlay → Chromecast**: Cast from iPhone/Mac to Chromecast speakers
2. **AirPlay → DLNA**: Bridge AirPlay to DLNA renderers
3. **AirPlay → AirPlay**: Forward between different AirPlay versions
4. **Multi-protocol bridging**: Any combination of protocols

## Recommendations

### Short Term
1. **Close issue as "investigated"** with findings documented
2. OR **Implement wrapper approach** using shairport-sync subprocess
   - Faster to implement
   - More reliable
   - Easier to maintain

### Long Term
1. **Monitor go.raopd** for updates or fork to fix build issues
2. **Consider contributing** to openairplay/goplay2 for better Go support
3. **Evaluate commercial libraries** if budget allows

## Dependencies Required

### For Wrapper Approach
- shairport-sync binary (3.3+ recommended)
- ALSA or PulseAudio (Linux)
- libao (macOS)

### For Native Go Approach  
- github.com/maghul/go.raopd (needs fixes)
- OR github.com/openairplay/goplay2 (AirPlay 2 only)
- External: libfdk-aac, PortAudio (for goplay2)

## Performance Considerations

- **Latency**: Expect 1-3 seconds total (RAOP buffer + forwarding + output)
- **CPU**: Moderate (decryption + ALAC decompression + optional transcoding)
- **Network**: Bandwidth ~1.4 Mbps for 44.1kHz 16-bit stereo PCM
- **Memory**: ~10-20 MB per active receiver

## Security Considerations

- Virtual receivers would be visible on network (like any AirPlay device)
- RAOP uses RSA/AES encryption (handled by library)
- No additional security risks vs existing AirPlay devices
- Should support auth/pairing if implementing AirPlay 2

## Testing Plan

1. **Unit tests**: Test receiver lifecycle, configuration
2. **Integration tests**: Test with actual AirPlay clients
3. **End-to-end tests**: Test full pipeline (receive → forward → play)
4. **Performance tests**: Measure latency, CPU usage
5. **Interop tests**: Test with various AirPlay sources (iOS, macOS, iTunes)

## Conclusion

Implementing virtual AirPlay receivers is **feasible but non-trivial**. The recommended approach is:

1. **Start with wrapper approach** (shairport-sync subprocess) for faster time-to-market
2. **Evaluate success** and user demand
3. **Consider native Go implementation** as phase 2 if needed

The wrapper approach provides 80% of the value with 20% of the effort, while native Go implementation would be cleaner but require significantly more development time.
