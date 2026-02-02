package client

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/hydragon2m/tunnel-agent/internal/logger"
	"github.com/hydragon2m/tunnel-agent/internal/metrics"
	v1 "github.com/hydragon2m/tunnel-protocol/go/v1"
)

// Dispatcher xử lý frames từ Core Server
type Dispatcher struct {
	conn   io.Reader
	connMu sync.RWMutex

	// Frame handlers
	controlHandler func(frame *v1.Frame) error
	streamHandler  func(frame *v1.Frame) error

	// State
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.RWMutex

	// Config
	readTimeout time.Duration
}

// NewDispatcher tạo Dispatcher mới
func NewDispatcher(readTimeout time.Duration) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())

	return &Dispatcher{
		readTimeout: readTimeout,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetConnection set connection để đọc frames
func (d *Dispatcher) SetConnection(conn io.Reader) {
	d.connMu.Lock()
	defer d.connMu.Unlock()
	d.conn = conn
}

// SetControlHandler set handler cho control frames
func (d *Dispatcher) SetControlHandler(handler func(frame *v1.Frame) error) {
	d.controlHandler = handler
}

// SetStreamHandler set handler cho stream frames
func (d *Dispatcher) SetStreamHandler(handler func(frame *v1.Frame) error) {
	d.streamHandler = handler
}

// Start bắt đầu frame reading loop
func (d *Dispatcher) Start() error {
	d.runningMu.Lock()
	if d.running {
		d.runningMu.Unlock()
		return ErrAlreadyRunning
	}
	d.running = true
	d.runningMu.Unlock()

	go d.readLoop()
	return nil
}

// Stop dừng frame reading loop
func (d *Dispatcher) Stop() {
	d.cancel()
	d.runningMu.Lock()
	d.running = false
	d.runningMu.Unlock()
}

// readLoop đọc frames liên tục
func (d *Dispatcher) readLoop() {
	for {
		select {
		case <-d.ctx.Done():
			return
		default:
		}

		// Get connection
		d.connMu.RLock()
		conn := d.conn
		d.connMu.RUnlock()

		if conn == nil {
			// Wait for connection
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Set read deadline if connection supports it
		if connWithDeadline, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			connWithDeadline.SetReadDeadline(time.Now().Add(d.readTimeout))
		}

		// Decode frame
		frame, err := v1.Decode(conn)
		if err != nil {
			if err == io.EOF {
				// Connection closed
				logger.Debug("Connection closed (EOF)")
				return
			}
			// Check if it's a timeout error (expected when no data is received)
			// Timeout errors contain "timeout" in the error message
			errStr := err.Error()
			if contains(errStr, "timeout") || contains(errStr, "i/o timeout") {
				// Timeout is expected when no data is received - continue reading
				// This keeps the connection alive even when idle
				logger.Debug("Read timeout (no data), continuing...")
				continue
			}
			// Other connection errors, return to trigger reconnection
			logger.Warn("Frame decode error", "error", err)
			metrics.GetMetrics().IncrementFramesError()
			return
		}

		// Track frame received
		metrics.GetMetrics().IncrementFramesReceived()

		// Handle frame
		if err := d.handleFrame(frame); err != nil {
			// Frame handling error, log but continue
			logger.Error("Frame handling error", "error", err, "type", frame.Type, "streamID", frame.StreamID)
			metrics.GetMetrics().IncrementFramesError()
			continue
		}
	}
}

// handleFrame xử lý frame
func (d *Dispatcher) handleFrame(frame *v1.Frame) error {
	// Control frames (StreamID = 0)
	if frame.IsControlFrame() {
		if d.controlHandler != nil {
			return d.controlHandler(frame)
		}
		return nil
	}

	// Data stream frames (StreamID > 0)
	if d.streamHandler != nil {
		return d.streamHandler(frame)
	}

	return nil
}

// IsRunning kiểm tra dispatcher có đang chạy không
func (d *Dispatcher) IsRunning() bool {
	d.runningMu.RLock()
	defer d.runningMu.RUnlock()
	return d.running
}

// contains checks if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
