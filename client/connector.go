package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	v1 "github.com/hydragon2m/tunnel-protocol/go/v1"
	"github.com/hydragon2m/tunnel-agent/internal/health"
	"github.com/hydragon2m/tunnel-agent/internal/logger"
	"github.com/hydragon2m/tunnel-agent/internal/metrics"
)

// Connector quản lý kết nối TLS tới Core Server
type Connector struct {
	serverAddr string
	tlsConfig  *tls.Config
	
	// Connection state
	conn     net.Conn
	connMu   sync.RWMutex
	connected bool
	
	// Reconnection
	maxRetries    int
	retryInterval time.Duration
	backoffFactor float64
	maxBackoff    time.Duration
	
	// Callbacks
	onConnected    func(conn net.Conn)
	onDisconnected func()
	onError        func(err error)
	
	// State
	ctx    context.Context
	cancel context.CancelFunc
}

// NewConnector tạo Connector mới
func NewConnector(serverAddr string, tlsConfig *tls.Config) *Connector {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Connector{
		serverAddr:    serverAddr,
		tlsConfig:     tlsConfig,
		maxRetries:    -1, // Unlimited
		retryInterval: 1 * time.Second,
		backoffFactor: 2.0,
		maxBackoff:    60 * time.Second,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetMaxRetries set max retry attempts (-1 = unlimited)
func (c *Connector) SetMaxRetries(maxRetries int) {
	c.maxRetries = maxRetries
}

// SetRetryInterval set retry interval
func (c *Connector) SetRetryInterval(interval time.Duration) {
	c.retryInterval = interval
}

// SetOnConnected set callback khi connected
func (c *Connector) SetOnConnected(callback func(conn net.Conn)) {
	c.onConnected = callback
}

// SetOnDisconnected set callback khi disconnected
func (c *Connector) SetOnDisconnected(callback func()) {
	c.onDisconnected = callback
}

// SetOnError set callback khi có error
func (c *Connector) SetOnError(callback func(err error)) {
	c.onError = callback
}

// Connect kết nối tới Core Server
func (c *Connector) Connect() error {
	return c.connectWithRetry()
}

// connectWithRetry kết nối với retry logic và improved error recovery
func (c *Connector) connectWithRetry() error {
	backoff := c.retryInterval
	retries := 0
	consecutiveErrors := 0
	maxConsecutiveErrors := 5
	
	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		default:
		}
		
		// Attempt connection
		conn, err := c.dial()
		if err == nil {
		// Connection successful - reset error counter
		consecutiveErrors = 0
		c.setConnection(conn)
		
		// Update metrics
		metrics.GetMetrics().IncrementConnectionsTotal()
		metrics.GetMetrics().IncrementConnectionsActive()
		metrics.GetMetrics().SetLastConnectionTime(time.Now())
		
		// Update health check
		if check, ok := health.GetHealthChecker().GetCheck("connection"); ok {
			check.UpdateCheck(health.HealthStatusHealthy, "Connected to server")
		}
		
		logger.Info("Connection established", "address", c.serverAddr)
		
		if c.onConnected != nil {
			c.onConnected(conn)
		}
		return nil
		}
		
		// Connection failed
		consecutiveErrors++
		
		// If too many consecutive errors, increase backoff more aggressively
		if consecutiveErrors >= maxConsecutiveErrors {
			backoff = time.Duration(float64(backoff) * c.backoffFactor * 1.5)
			if backoff > c.maxBackoff*2 {
				backoff = c.maxBackoff * 2
			}
		}
		
		if c.onError != nil {
			c.onError(fmt.Errorf("connection failed (retry %d/%d): %w", retries+1, c.maxRetries, err))
		}
		
		// Check max retries
		if c.maxRetries > 0 && retries >= c.maxRetries {
			return fmt.Errorf("max retries exceeded after %d attempts: %w", retries, err)
		}
		
		retries++
		
		// Wait before retry
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(backoff):
			// Exponential backoff
			backoff = time.Duration(float64(backoff) * c.backoffFactor)
			if backoff > c.maxBackoff {
				backoff = c.maxBackoff
			}
		}
	}
}

// dial tạo TLS connection
func (c *Connector) dial() (net.Conn, error) {
	if c.tlsConfig != nil {
		return tls.Dial("tcp", c.serverAddr, c.tlsConfig)
	}
	return net.Dial("tcp", c.serverAddr)
}

// setConnection set connection và update state
func (c *Connector) setConnection(conn net.Conn) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	
	c.conn = conn
	c.connected = true
}

// GetConnection lấy connection hiện tại
func (c *Connector) GetConnection() (net.Conn, bool) {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	
	return c.conn, c.connected
}

// IsConnected kiểm tra connection status
func (c *Connector) IsConnected() bool {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	
	return c.connected
}

// Disconnect ngắt kết nối
func (c *Connector) Disconnect() error {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	
	if c.conn == nil {
		return nil
	}
	
	err := c.conn.Close()
	c.conn = nil
	c.connected = false
	
	// Update metrics
	metrics.GetMetrics().DecrementConnectionsActive()
	
	// Update health check
	if check, ok := health.GetHealthChecker().GetCheck("connection"); ok {
		check.UpdateCheck(health.HealthStatusUnhealthy, "Disconnected from server")
	}
	
	logger.Info("Connection closed")
	
	if c.onDisconnected != nil {
		c.onDisconnected()
	}
	
	return err
}

// Reconnect ngắt kết nối và kết nối lại
func (c *Connector) Reconnect() error {
	logger.Info("Reconnecting to server")
	metrics.GetMetrics().IncrementReconnectionsTotal()
	
	c.Disconnect()
	
	err := c.connectWithRetry()
	if err != nil {
		metrics.GetMetrics().IncrementReconnectionErrors()
		logger.Error("Reconnection failed", "error", err)
	} else {
		logger.Info("Reconnection successful")
	}
	
	return err
}

// Close đóng connector
func (c *Connector) Close() error {
	c.cancel()
	return c.Disconnect()
}

// SendFrame gửi frame qua connection
func (c *Connector) SendFrame(frame *v1.Frame) error {
	c.connMu.RLock()
	conn := c.conn
	connected := c.connected
	c.connMu.RUnlock()
	
	if !connected || conn == nil {
		return ErrNotConnected
	}
	
	err := v1.Encode(conn, frame)
	if err == nil {
		metrics.GetMetrics().IncrementFramesSent()
	} else {
		logger.Warn("Failed to send frame", "error", err, "type", frame.Type)
	}
	
	return err
}

// Context returns context for cancellation
func (c *Connector) Context() context.Context {
	return c.ctx
}

