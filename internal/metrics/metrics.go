package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects agent metrics
type Metrics struct {
	// Connection metrics
	ConnectionsTotal    int64
	ConnectionsActive   int64
	ReconnectionsTotal  int64
	ReconnectionErrors  int64
	
	// Stream metrics
	StreamsTotal        int64
	StreamsActive       int64
	StreamsCompleted   int64
	StreamsFailed      int64
	
	// Request metrics
	RequestsTotal       int64
	RequestsSuccess    int64
	RequestsFailed     int64
	RequestDuration    int64 // microseconds
	
	// Frame metrics
	FramesReceived      int64
	FramesSent          int64
	FramesError         int64
	
	// Heartbeat metrics
	HeartbeatsSent      int64
	HeartbeatsFailed   int64
	
	// Local service metrics
	LocalRequestsTotal  int64
	LocalRequestsError  int64
	LocalRequestDuration int64 // microseconds
	
	// Timestamps
	LastConnectionTime  time.Time
	LastRequestTime     time.Time
	LastHeartbeatTime   time.Time
	
	mu sync.RWMutex
}

var (
	// Global metrics instance
	globalMetrics = &Metrics{}
)

// GetMetrics returns global metrics instance
func GetMetrics() *Metrics {
	return globalMetrics
}

// IncrementConnectionsTotal increments total connections
func (m *Metrics) IncrementConnectionsTotal() {
	atomic.AddInt64(&m.ConnectionsTotal, 1)
}

// IncrementConnectionsActive increments active connections
func (m *Metrics) IncrementConnectionsActive() {
	atomic.AddInt64(&m.ConnectionsActive, 1)
}

// DecrementConnectionsActive decrements active connections
func (m *Metrics) DecrementConnectionsActive() {
	atomic.AddInt64(&m.ConnectionsActive, -1)
}

// IncrementReconnectionsTotal increments total reconnections
func (m *Metrics) IncrementReconnectionsTotal() {
	atomic.AddInt64(&m.ReconnectionsTotal, 1)
}

// IncrementReconnectionErrors increments reconnection errors
func (m *Metrics) IncrementReconnectionErrors() {
	atomic.AddInt64(&m.ReconnectionErrors, 1)
}

// IncrementStreamsTotal increments total streams
func (m *Metrics) IncrementStreamsTotal() {
	atomic.AddInt64(&m.StreamsTotal, 1)
}

// IncrementStreamsActive increments active streams
func (m *Metrics) IncrementStreamsActive() {
	atomic.AddInt64(&m.StreamsActive, 1)
}

// DecrementStreamsActive decrements active streams
func (m *Metrics) DecrementStreamsActive() {
	atomic.AddInt64(&m.StreamsActive, -1)
}

// IncrementStreamsCompleted increments completed streams
func (m *Metrics) IncrementStreamsCompleted() {
	atomic.AddInt64(&m.StreamsCompleted, 1)
}

// IncrementStreamsFailed increments failed streams
func (m *Metrics) IncrementStreamsFailed() {
	atomic.AddInt64(&m.StreamsFailed, 1)
}

// IncrementRequestsTotal increments total requests
func (m *Metrics) IncrementRequestsTotal() {
	atomic.AddInt64(&m.RequestsTotal, 1)
}

// IncrementRequestsSuccess increments successful requests
func (m *Metrics) IncrementRequestsSuccess() {
	atomic.AddInt64(&m.RequestsSuccess, 1)
}

// IncrementRequestsFailed increments failed requests
func (m *Metrics) IncrementRequestsFailed() {
	atomic.AddInt64(&m.RequestsFailed, 1)
}

// RecordRequestDuration records request duration
func (m *Metrics) RecordRequestDuration(duration time.Duration) {
	atomic.StoreInt64(&m.RequestDuration, duration.Microseconds())
}

// IncrementFramesReceived increments received frames
func (m *Metrics) IncrementFramesReceived() {
	atomic.AddInt64(&m.FramesReceived, 1)
}

// IncrementFramesSent increments sent frames
func (m *Metrics) IncrementFramesSent() {
	atomic.AddInt64(&m.FramesSent, 1)
}

// IncrementFramesError increments error frames
func (m *Metrics) IncrementFramesError() {
	atomic.AddInt64(&m.FramesError, 1)
}

// IncrementHeartbeatsSent increments sent heartbeats
func (m *Metrics) IncrementHeartbeatsSent() {
	atomic.AddInt64(&m.HeartbeatsSent, 1)
}

// IncrementHeartbeatsFailed increments failed heartbeats
func (m *Metrics) IncrementHeartbeatsFailed() {
	atomic.AddInt64(&m.HeartbeatsFailed, 1)
}

// IncrementLocalRequestsTotal increments total local requests
func (m *Metrics) IncrementLocalRequestsTotal() {
	atomic.AddInt64(&m.LocalRequestsTotal, 1)
}

// IncrementLocalRequestsError increments local request errors
func (m *Metrics) IncrementLocalRequestsError() {
	atomic.AddInt64(&m.LocalRequestsError, 1)
}

// RecordLocalRequestDuration records local request duration
func (m *Metrics) RecordLocalRequestDuration(duration time.Duration) {
	atomic.StoreInt64(&m.LocalRequestDuration, duration.Microseconds())
}

// SetLastConnectionTime sets last connection time
func (m *Metrics) SetLastConnectionTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastConnectionTime = t
}

// SetLastRequestTime sets last request time
func (m *Metrics) SetLastRequestTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastRequestTime = t
}

// SetLastHeartbeatTime sets last heartbeat time
func (m *Metrics) SetLastHeartbeatTime(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.LastHeartbeatTime = t
}

// GetSnapshot returns metrics snapshot
func (m *Metrics) GetSnapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return MetricsSnapshot{
		ConnectionsTotal:     atomic.LoadInt64(&m.ConnectionsTotal),
		ConnectionsActive:    atomic.LoadInt64(&m.ConnectionsActive),
		ReconnectionsTotal:   atomic.LoadInt64(&m.ReconnectionsTotal),
		ReconnectionErrors:   atomic.LoadInt64(&m.ReconnectionErrors),
		StreamsTotal:        atomic.LoadInt64(&m.StreamsTotal),
		StreamsActive:       atomic.LoadInt64(&m.StreamsActive),
		StreamsCompleted:   atomic.LoadInt64(&m.StreamsCompleted),
		StreamsFailed:       atomic.LoadInt64(&m.StreamsFailed),
		RequestsTotal:       atomic.LoadInt64(&m.RequestsTotal),
		RequestsSuccess:    atomic.LoadInt64(&m.RequestsSuccess),
		RequestsFailed:     atomic.LoadInt64(&m.RequestsFailed),
		RequestDuration:    atomic.LoadInt64(&m.RequestDuration),
		FramesReceived:     atomic.LoadInt64(&m.FramesReceived),
		FramesSent:         atomic.LoadInt64(&m.FramesSent),
		FramesError:        atomic.LoadInt64(&m.FramesError),
		HeartbeatsSent:     atomic.LoadInt64(&m.HeartbeatsSent),
		HeartbeatsFailed:   atomic.LoadInt64(&m.HeartbeatsFailed),
		LocalRequestsTotal: atomic.LoadInt64(&m.LocalRequestsTotal),
		LocalRequestsError: atomic.LoadInt64(&m.LocalRequestsError),
		LocalRequestDuration: atomic.LoadInt64(&m.LocalRequestDuration),
		LastConnectionTime:  m.LastConnectionTime,
		LastRequestTime:     m.LastRequestTime,
		LastHeartbeatTime:   m.LastHeartbeatTime,
	}
}

// MetricsSnapshot is a snapshot of metrics
type MetricsSnapshot struct {
	ConnectionsTotal      int64
	ConnectionsActive     int64
	ReconnectionsTotal    int64
	ReconnectionErrors    int64
	StreamsTotal          int64
	StreamsActive         int64
	StreamsCompleted      int64
	StreamsFailed         int64
	RequestsTotal         int64
	RequestsSuccess       int64
	RequestsFailed        int64
	RequestDuration       int64
	FramesReceived        int64
	FramesSent            int64
	FramesError           int64
	HeartbeatsSent        int64
	HeartbeatsFailed      int64
	LocalRequestsTotal    int64
	LocalRequestsError    int64
	LocalRequestDuration  int64
	LastConnectionTime    time.Time
	LastRequestTime       time.Time
	LastHeartbeatTime     time.Time
}

