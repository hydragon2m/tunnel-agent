package client

import (
	"sync"
	"time"
)

// Stream đại diện cho 1 stream từ Core Server
type Stream struct {
	ID        uint32
	State     StreamState
	CreatedAt time.Time
	Metadata  map[string]string
	
	// Data channels
	dataOut   chan []byte
	closeCh   chan struct{}
	
	mu sync.RWMutex
}

// StreamState là state của stream
type StreamState int

const (
	StreamStateInit StreamState = iota
	StreamStateOpen
	StreamStateData
	StreamStateClosed
	StreamStateError
)

// StreamManager quản lý streams
type StreamManager struct {
	streams   map[uint32]*Stream
	streamsMu sync.RWMutex
	
	// Callbacks
	onStreamCreated func(streamID uint32)
	onStreamClosed  func(streamID uint32)
}

// NewStreamManager tạo StreamManager mới
func NewStreamManager() *StreamManager {
	return &StreamManager{
		streams: make(map[uint32]*Stream),
	}
}

// SetOnStreamCreated set callback khi stream được tạo
func (sm *StreamManager) SetOnStreamCreated(callback func(streamID uint32)) {
	sm.onStreamCreated = callback
}

// SetOnStreamClosed set callback khi stream đóng
func (sm *StreamManager) SetOnStreamClosed(callback func(streamID uint32)) {
	sm.onStreamClosed = callback
}

// CreateStream tạo stream mới
func (sm *StreamManager) CreateStream(streamID uint32) (*Stream, error) {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()
	
	if _, exists := sm.streams[streamID]; exists {
		return nil, ErrStreamAlreadyExists
	}
	
	stream := &Stream{
		ID:        streamID,
		State:     StreamStateInit,
		CreatedAt: time.Now(),
		Metadata:  make(map[string]string),
		dataOut:   make(chan []byte, 10),
		closeCh:   make(chan struct{}),
	}
	
	sm.streams[streamID] = stream
	
	if sm.onStreamCreated != nil {
		sm.onStreamCreated(streamID)
	}
	
	return stream, nil
}

// GetStream lấy stream theo ID
func (sm *StreamManager) GetStream(streamID uint32) (*Stream, bool) {
	sm.streamsMu.RLock()
	defer sm.streamsMu.RUnlock()
	
	stream, ok := sm.streams[streamID]
	return stream, ok
}

// CloseStream đóng stream
func (sm *StreamManager) CloseStream(streamID uint32) error {
	sm.streamsMu.Lock()
	defer sm.streamsMu.Unlock()
	
	stream, exists := sm.streams[streamID]
	if !exists {
		return ErrStreamNotFound
	}
	
	stream.setState(StreamStateClosed)
	close(stream.closeCh)
	// Note: Don't close dataOut channel here as it might be used by other goroutines
	// Channel will be garbage collected when stream is deleted
	delete(sm.streams, streamID)
	
	if sm.onStreamClosed != nil {
		sm.onStreamClosed(streamID)
	}
	
	return nil
}

// setState set state của stream
func (s *Stream) setState(state StreamState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.State = state
}

// GetState lấy state của stream
func (s *Stream) GetState() StreamState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// DataOut returns data output channel
func (s *Stream) DataOut() chan<- []byte {
	return s.dataOut
}

// CloseCh returns close channel
func (s *Stream) CloseCh() <-chan struct{} {
	return s.closeCh
}

// SetMetadata set metadata
func (s *Stream) SetMetadata(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}
	s.Metadata[key] = value
}

// GetMetadata lấy metadata
func (s *Stream) GetMetadata(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Metadata == nil {
		return "", false
	}
	value, ok := s.Metadata[key]
	return value, ok
}

