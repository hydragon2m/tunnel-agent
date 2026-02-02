package client

import (
	"context"
	"time"

	"github.com/hydragon2m/tunnel-agent/internal/logger"
	"github.com/hydragon2m/tunnel-agent/internal/metrics"
	v1 "github.com/hydragon2m/tunnel-protocol/go/v1"
)

// Heartbeat gửi periodic heartbeat đến Core Server
type Heartbeat struct {
	connector *Connector
	interval  time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
}

// NewHeartbeat tạo Heartbeat mới
func NewHeartbeat(connector *Connector, interval time.Duration) *Heartbeat {
	ctx, cancel := context.WithCancel(context.Background())

	return &Heartbeat{
		connector: connector,
		interval:  interval,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start bắt đầu heartbeat loop
func (h *Heartbeat) Start() {
	if h.running {
		return
	}
	h.running = true

	go h.heartbeatLoop()
}

// Stop dừng heartbeat loop
func (h *Heartbeat) Stop() {
	h.cancel()
	h.running = false
}

// heartbeatLoop gửi heartbeat định kỳ
func (h *Heartbeat) heartbeatLoop() {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			// Send heartbeat
			if h.connector.IsConnected() {
				frame := &v1.Frame{
					Version:  v1.Version,
					Type:     v1.FrameHeartbeat,
					Flags:    v1.FlagNone,
					StreamID: v1.StreamIDControl,
					Payload:  nil,
				}

				err := h.connector.SendFrame(frame)
				if err != nil {
					metrics.GetMetrics().IncrementHeartbeatsFailed()
					logger.Warn("Heartbeat send failed", "error", err)
				} else {
					metrics.GetMetrics().IncrementHeartbeatsSent()
					metrics.GetMetrics().SetLastHeartbeatTime(time.Now())
				}
			}
		}
	}
}
