// Package state manages the playback state for the DLNA renderer.
package state

import (
	"context"
	"sync"
)

// TransportState represents the current playback state.
type TransportState string

const (
	StateStopped    TransportState = "STOPPED"
	StatePlaying    TransportState = "PLAYING"
	StatePaused     TransportState = "PAUSED_PLAYBACK"
	StateTransition TransportState = "TRANSITIONING"
	StateNoMedia    TransportState = "NO_MEDIA_PRESENT"
)

// PlayerState holds the current state of the DLNA renderer.
type PlayerState struct {
	mu sync.RWMutex

	ctx       context.Context
	cancel    context.CancelFunc
	
	// Session management
	sessionOwner string

	// Transport state
	transportState TransportState
	uri            string
	metadata       string

	// Volume state
	volume int
	muted  bool
}

// New creates a new PlayerState.
func New(ctx context.Context) *PlayerState {
	ctx, cancel := context.WithCancel(ctx)
	return &PlayerState{
		ctx:            ctx,
		cancel:         cancel,
		transportState: StateNoMedia,
		volume:         80,
	}
}

// Context returns the state's context.
func (s *PlayerState) Context() context.Context {
	return s.ctx
}

// Stop cancels the state's context.
func (s *PlayerState) Stop() {
	s.cancel()
}

// AcquireOrCheckSession attempts to acquire or verify session ownership.
func (s *PlayerState) AcquireOrCheckSession(controller string, allowPreempt bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sessionOwner == "" || s.sessionOwner == controller {
		s.sessionOwner = controller
		return true
	}
	if allowPreempt {
		s.sessionOwner = controller
		return true
	}
	return false
}

// HasSession checks if the controller owns the session.
func (s *PlayerState) HasSession(controller string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionOwner == "" || s.sessionOwner == controller
}

// ReleaseSession clears the session owner.
func (s *PlayerState) ReleaseSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionOwner = ""
}

// SetURI sets the current playback URI and metadata.
func (s *PlayerState) SetURI(uri, metadata string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uri = uri
	s.metadata = metadata
	s.transportState = StateStopped
}

// GetURI returns the current URI and metadata.
func (s *PlayerState) GetURI() (string, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.uri, s.metadata
}

// SetTransportState sets the transport state.
func (s *PlayerState) SetTransportState(state TransportState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transportState = state
}

// GetTransportState returns the current transport state.
func (s *PlayerState) GetTransportState() TransportState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.transportState
}

// SetVolume sets the volume (0-100).
func (s *PlayerState) SetVolume(v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	s.volume = v
}

// GetVolume returns the current volume.
func (s *PlayerState) GetVolume() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.volume
}

// SetMute sets the mute state.
func (s *PlayerState) SetMute(m bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.muted = m
}

// GetMute returns the mute state.
func (s *PlayerState) GetMute() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.muted
}

// Clear resets the playback state.
func (s *PlayerState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uri = ""
	s.metadata = ""
	s.transportState = StateNoMedia
	s.sessionOwner = ""
}
