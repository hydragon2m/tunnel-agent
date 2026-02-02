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

		// Set read deadline if connection supports it
		if connWithDeadline, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
			connWithDeadline.SetReadDeadline(time.Now().Add(d.readTimeout))
		}

		// 1. Read Frame Length
		length, err := v1.ReadFrameLength(conn)
		if err != nil {
			if err == io.EOF {
				logger.Debug("Connection closed (EOF)")
				return
			}
			// Check timeout
			if contains(err.Error(), "timeout") {
				logger.Debug("Read timeout (no data), continuing...")
				continue
			}
			logger.Warn("Frame length read error", "error", err)
			metrics.GetMetrics().IncrementFramesError()
			return
		}

		// 2. Validate Length (optional check before allocation, ParseFrame also checks but better here)
		if length < v1.HeaderSize || length > v1.MaxFrameSize {
			logger.Warn("Invalid frame size", "length", length)
			metrics.GetMetrics().IncrementFramesError()
			// Consume/discard? Or just close connection? Safe to close.
			return
		}

		// 3. Get Buffer from Pool
		// We need 'length' bytes.
		buf := v1.GetBuffer(int(length))

		// 4. Read the rest of the frame (Magic + Header + StreamID + Payload)
		// Note: buf might be larger than length. We read into buf[:length]
		if _, err := io.ReadFull(conn, buf[:length]); err != nil {
			logger.Warn("Frame body read error", "error", err)
			v1.PutBuffer(buf) // Return buffer on error
			return
		}

		// 5. Parse Frame
		// ParseFrame uses the buffer content.
		// BE CAREFUL: The returned frame.Payload points into 'buf'.
		// We must ENSURE we don't return 'buf' to the pool while it's still being used.
		// Since we handle the frame synchronously in handleFrame and it likely copies
		// data if needed (e.g. to channel), we need to enforce that handlers don't hold the payload slice.
		// IF handleFrame blocks or queues the frame pointer, we have a race/corruption if we PutBuffer here.
		//
		// For streamHandler which passes frame.Payload to channel: `stream.dataIn <- frame.Payload`
		// This means the receiver owns the slice. If we pool it, we can't reuse it until receiver is done.
		//
		// DECISION: For now, to support zero-copy where possible but safety first:
		// We should COPY the payload if we want to return the buffer immediately.
		// OR we rely on GC for now (don't PutBuffer) if we can't guarantee lifecycle.
		//
		// OPTIMIZATION: `ParseFrame` returns a Frame struct.
		// If we use `GetBuffer`, we MUST `Copy` the payload if we want to `PutBuffer` back in this loop.
		//
		// Let's modify logic:
		// ParseFrame creates *Frame referencing buf.
		// If we want to pool, we need to copy payload.
		// `frame.Payload = append([]byte(nil), frame.Payload...)`
		// Then `v1.PutBuffer(buf)`
		//
		// BUT: This defeats the purpose of zero allocation for the Payload?
		// Correct. The goal was to reuse buffer for reading from network.
		//
		// Actually, `tunnel-core` does `stream.dataIn <- frame.Payload`.
		// If `stream.dataIn` is buffered, the payload sits there.
		//
		// COMPROMISE: For this optimization step, we'll implement the Pool reading side,
		// but we will NOT return the buffer to the pool if it successfully parsed,
		// effectively falling back to GC for valid frames (but using pool for read buffer allocation?).
		// Wait, if we don't Put back, we just allocated from pool and lost it to GC.
		// That's fine if pool refills with `make`.
		// But `sync.Pool` requires Put to be useful.
		//
		// REAL OPTIMIZATION: `handleStreamFrame` should copy data it needs if it wants to keep it?
		// Or we implement a ref-counted buffer? Too complex.
		//
		// Let's just copy the payload for now. Even with copy, we save the allocation of the *initial read buffer*
		// if we can reuse it for cases where we don't need to keep payload (e.g. control frames, or short frames).
		//
		// ACTUALLY: `ParseFrame` returns `Payload` as slice of `buf`.
		// Let's copy it immediately so we can return `buf` to pool.
		frame, err := v1.ParseFrame(buf[:length])
		if err != nil {
			logger.Warn("Frame parse error", "error", err)
			v1.PutBuffer(buf)
			metrics.GetMetrics().IncrementFramesError()
			return
		}

		// Copy payload so we can reuse buffer
		// Only needed if Payload has data
		if len(frame.Payload) > 0 {
			newPayload := make([]byte, len(frame.Payload))
			copy(newPayload, frame.Payload)
			frame.Payload = newPayload
		}

		// Now we can safe return buf
		v1.PutBuffer(buf)

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
