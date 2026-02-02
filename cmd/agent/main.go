package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/hydragon2m/tunnel-agent/client"
	"github.com/hydragon2m/tunnel-agent/internal/health"
	"github.com/hydragon2m/tunnel-agent/internal/logger"
	"github.com/hydragon2m/tunnel-agent/internal/metrics"
	v1 "github.com/hydragon2m/tunnel-protocol/go/v1"
)

var (
	// Server config
	serverAddr = flag.String("server", "localhost:8443", "Core server address")
	useTLS     = flag.Bool("tls", true, "Use TLS connection")
	skipVerify = flag.Bool("skip-verify", false, "Skip TLS certificate verification")

	// Auth config
	token   = flag.String("token", "", "Authentication token (required)")
	agentID = flag.String("agent-id", "", "Agent ID (optional)")
	version = flag.String("version", "1.0.0", "Agent version")

	// Local service config
	localURL = flag.String("local", "http://localhost:3003", "Local service URL")

	// Config
	heartbeatInterval = flag.Duration("heartbeat", 10*time.Second, "Heartbeat interval")
	readTimeout       = flag.Duration("read-timeout", 30*time.Second, "Read timeout")
	requestTimeout    = flag.Duration("request-timeout", 30*time.Second, "Request timeout")

	// Logging
	logLevel = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logJSON  = flag.Bool("log-json", false, "Use JSON logging format")

	// Metrics
	metricsEnabled = flag.Bool("metrics", false, "Enable metrics collection")
	metricsPort    = flag.Int("metrics-port", 9091, "Metrics HTTP server port")
)

func main() {
	flag.Parse()

	// Override with environment variables if set
	if envServer := os.Getenv("SERVER"); envServer != "" {
		*serverAddr = envServer
	}
	if envTLS := os.Getenv("TLS"); envTLS != "" {
		*useTLS = (envTLS == "true")
	}
	if envSkipVerify := os.Getenv("SKIP_VERIFY"); envSkipVerify != "" {
		*skipVerify = (envSkipVerify == "true")
	}
	if envToken := os.Getenv("TOKEN"); envToken != "" {
		*token = envToken
	}
	if envAgentID := os.Getenv("AGENT_ID"); envAgentID != "" {
		*agentID = envAgentID
	}
	if envLocal := os.Getenv("LOCAL"); envLocal != "" {
		*localURL = envLocal
	}
	if envHeartbeat := os.Getenv("HEARTBEAT"); envHeartbeat != "" {
		if duration, err := time.ParseDuration(envHeartbeat); err == nil {
			*heartbeatInterval = duration
		}
	}
	if envReadTimeout := os.Getenv("READ_TIMEOUT"); envReadTimeout != "" {
		if duration, err := time.ParseDuration(envReadTimeout); err == nil {
			*readTimeout = duration
		}
	}
	if envRequestTimeout := os.Getenv("REQUEST_TIMEOUT"); envRequestTimeout != "" {
		if duration, err := time.ParseDuration(envRequestTimeout); err == nil {
			*requestTimeout = duration
		}
	}
	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		*logLevel = envLogLevel
	}
	if envLogJSON := os.Getenv("LOG_JSON"); envLogJSON != "" {
		*logJSON = (envLogJSON == "true")
	}
	if envMetrics := os.Getenv("METRICS"); envMetrics != "" {
		*metricsEnabled = (envMetrics == "true")
	}
	if envMetricsPort := os.Getenv("METRICS_PORT"); envMetricsPort != "" {
		if port, err := parseInt(envMetricsPort); err == nil {
			*metricsPort = port
		}
	}

	if *token == "" {
		log.Fatal("Token is required. Use -token flag or TOKEN environment variable")
	}

	// Initialize structured logging
	logger.InitLogger(*logLevel, *logJSON)
	logger.Info("Starting Tunnel Agent", "version", *version, "agentID", *agentID)

	// Initialize health checks
	healthChecker := health.GetHealthChecker()
	connectionCheck := healthChecker.RegisterCheck("connection")
	connectionCheck.UpdateCheck(health.HealthStatusDegraded, "Not connected")

	streamCheck := healthChecker.RegisterCheck("streams")
	streamCheck.UpdateCheck(health.HealthStatusHealthy, "No active streams")

	localServiceCheck := healthChecker.RegisterCheck("local_service")
	localServiceCheck.UpdateCheck(health.HealthStatusHealthy, "Local service available")

	// Start metrics server if enabled
	if *metricsEnabled {
		go startMetricsServer(*metricsPort)
		logger.Info("Metrics server started", "port", *metricsPort)
	}

	// Create TLS config
	var tlsConfig *tls.Config
	if *useTLS {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: *skipVerify,
		}
	}

	// Create connector
	connector := client.NewConnector(*serverAddr, tlsConfig)
	connector.SetRetryInterval(1 * time.Second)

	// Create dispatcher
	dispatcher := client.NewDispatcher(*readTimeout)

	// Create stream manager
	streamManager := client.NewStreamManager()

	// Create local forwarder
	forwarder := client.NewLocalForwarder(*localURL, *requestTimeout)

	// Create authenticator
	authenticator := client.NewAuthenticator(*token, *agentID, *version, nil, nil)

	// Create heartbeat
	heartbeat := client.NewHeartbeat(connector, *heartbeatInterval)

	// Setup connection callbacks
	connector.SetOnConnected(func(conn net.Conn) {
		log.Printf("Connected to server: %s", *serverAddr)

		// Set connection for dispatcher
		dispatcher.SetConnection(conn)

		// Start dispatcher
		if err := dispatcher.Start(); err != nil {
			log.Printf("Failed to start dispatcher: %v", err)
			return
		}

		// Send authentication
		authFrame, err := authenticator.CreateAuthFrame()
		if err != nil {
			log.Printf("Failed to create auth frame: %v", err)
			return
		}

		if err := connector.SendFrame(authFrame); err != nil {
			log.Printf("Failed to send auth frame: %v", err)
			return
		}

		log.Println("Authentication frame sent")
	})

	connector.SetOnDisconnected(func() {
		log.Println("Disconnected from server")
		dispatcher.Stop()
	})

	connector.SetOnError(func(err error) {
		log.Printf("Connection error: %v", err)
	})

	// Setup dispatcher handlers
	dispatcher.SetControlHandler(func(frame *v1.Frame) error {
		switch frame.Type {
		case v1.FrameAuth:
			// Handle auth response
			if err := authenticator.HandleAuthResponse(frame); err != nil {
				logger.Error("Authentication failed", "error", err)
				connectionCheck.UpdateCheck(health.HealthStatusUnhealthy, "Authentication failed")
				return err
			}
			logger.Info("Authentication successful")
			connectionCheck.UpdateCheck(health.HealthStatusHealthy, "Authenticated")
			// Start heartbeat
			heartbeat.Start()

		case v1.FrameHeartbeat:
			// Heartbeat ACK, do nothing
			logger.Debug("Heartbeat ACK received")

		case v1.FrameClose:
			// Server wants to close connection
			logger.Info("Server requested connection close")
			connectionCheck.UpdateCheck(health.HealthStatusUnhealthy, "Server requested close")
			connector.Disconnect()

		default:
			logger.Warn("Unknown control frame type", "type", frame.Type)
		}
		return nil
	})

	dispatcher.SetStreamHandler(func(frame *v1.Frame) error {
		return handleStreamFrame(frame, streamManager, forwarder, connector, localServiceCheck)
	})

	// Setup stream manager callbacks
	streamManager.SetOnStreamCreated(func(streamID uint32) {
		logger.Info("Stream created", "streamID", streamID)
		metrics.GetMetrics().IncrementStreamsTotal()
		metrics.GetMetrics().IncrementStreamsActive()
		streamCheck.UpdateCheck(health.HealthStatusHealthy, "Streams active")
	})

	streamManager.SetOnStreamClosed(func(streamID uint32) {
		logger.Info("Stream closed", "streamID", streamID)
		metrics.GetMetrics().DecrementStreamsActive()
		metrics.GetMetrics().IncrementStreamsCompleted()
		if metrics.GetMetrics().GetSnapshot().StreamsActive == 0 {
			streamCheck.UpdateCheck(health.HealthStatusHealthy, "No active streams")
		}
	})

	// Connect to server
	logger.Info("Connecting to server", "address", *serverAddr, "tls", *useTLS)
	if err := connector.Connect(); err != nil {
		logger.Error("Failed to connect", "error", err)
		log.Fatalf("Failed to connect: %v", err)
	}

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	logger.Info("Agent started", "press", "Ctrl+C to stop")
	<-sigCh

	logger.Info("Shutting down...")

	// Send Close Frame
	closeFrame := &v1.Frame{
		Version:  v1.Version,
		Type:     v1.FrameClose,
		Flags:    v1.FlagNone,
		StreamID: v1.StreamIDControl,
	}
	if err := connector.SendFrame(closeFrame); err != nil {
		logger.Warn("Failed to send close frame", "error", err)
	}

	// Give some time for the write buffer to flush (writeLoop interval is 10ms)
	time.Sleep(100 * time.Millisecond)

	// Stop heartbeat
	heartbeat.Stop()

	// Stop dispatcher
	dispatcher.Stop()

	// Disconnect
	connector.Close()

	logger.Info("Shutdown complete")
}

// startMetricsServer starts HTTP server for metrics
func startMetricsServer(port int) {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		snapshot := metrics.GetMetrics().GetSnapshot()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
  "connections": {
    "total": %d,
    "active": %d,
    "reconnections": %d,
    "reconnection_errors": %d
  },
  "streams": {
    "total": %d,
    "active": %d,
    "completed": %d,
    "failed": %d
  },
  "requests": {
    "total": %d,
    "success": %d,
    "failed": %d,
    "duration_us": %d
  },
  "frames": {
    "received": %d,
    "sent": %d,
    "errors": %d
  },
  "heartbeat": {
    "sent": %d,
    "failed": %d
  },
  "local_service": {
    "requests_total": %d,
    "requests_error": %d,
    "duration_us": %d
  },
  "timestamps": {
    "last_connection": "%s",
    "last_request": "%s",
    "last_heartbeat": "%s"
  },
  "health": {
    "status": "%s"
  }
}`,
			snapshot.ConnectionsTotal,
			snapshot.ConnectionsActive,
			snapshot.ReconnectionsTotal,
			snapshot.ReconnectionErrors,
			snapshot.StreamsTotal,
			snapshot.StreamsActive,
			snapshot.StreamsCompleted,
			snapshot.StreamsFailed,
			snapshot.RequestsTotal,
			snapshot.RequestsSuccess,
			snapshot.RequestsFailed,
			snapshot.RequestDuration,
			snapshot.FramesReceived,
			snapshot.FramesSent,
			snapshot.FramesError,
			snapshot.HeartbeatsSent,
			snapshot.HeartbeatsFailed,
			snapshot.LocalRequestsTotal,
			snapshot.LocalRequestsError,
			snapshot.LocalRequestDuration,
			snapshot.LastConnectionTime.Format(time.RFC3339),
			snapshot.LastRequestTime.Format(time.RFC3339),
			snapshot.LastHeartbeatTime.Format(time.RFC3339),
			health.GetHealthChecker().GetOverallStatus(),
		)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		status := health.GetHealthChecker().GetOverallStatus()
		checks := health.GetHealthChecker().GetAllChecks()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
  "status": "%s",
  "checks": {`,
			status)

		first := true
		for name, check := range checks {
			if !first {
				fmt.Fprint(w, ",")
			}
			first = false
			checkStatus, message, lastCheck := check.GetStatus()
			fmt.Fprintf(w, `
    "%s": {
      "status": "%s",
      "message": "%s",
      "last_check": "%s"
    }`,
				name, checkStatus, message, lastCheck.Format(time.RFC3339))
		}

		fmt.Fprint(w, `
  }
}`)
	})

	addr := fmt.Sprintf(":%d", port)
	logger.Info("Metrics server listening", "address", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		logger.Error("Metrics server error", "error", err)
	}
}

// handleStreamFrame xử lý stream frames
func handleStreamFrame(
	frame *v1.Frame,
	streamManager *client.StreamManager,
	forwarder *client.LocalForwarder,
	connector *client.Connector,
	localServiceCheck *health.Check,
) error {
	switch frame.Type {
	case v1.FrameOpenStream:
		// Create new stream
		stream, err := streamManager.CreateStream(frame.StreamID)
		if err != nil {
			return fmt.Errorf("failed to create stream: %w", err)
		}

		// Forward request to local service in goroutine
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), *requestTimeout)
			defer cancel()

			responseData, err := forwarder.ForwardRequest(ctx, stream, frame.Payload)
			if err != nil {
				logger.Error("Failed to forward request", "error", err, "streamID", frame.StreamID)
				metrics.GetMetrics().IncrementStreamsFailed()
				localServiceCheck.UpdateCheck(health.HealthStatusDegraded, err.Error())

				// Send error frame (using FrameData with FlagError)
				errorFrame := &v1.Frame{
					Version:  v1.Version,
					Type:     v1.FrameData,
					Flags:    v1.FlagError,
					StreamID: frame.StreamID,
					Payload:  []byte(err.Error()),
				}
				_ = connector.SendFrame(errorFrame)
				streamManager.CloseStream(frame.StreamID)
				return
			}

			// Update health check on success
			localServiceCheck.UpdateCheck(health.HealthStatusHealthy, "Local service responding")

			// Send response
			dataFrame := &v1.Frame{
				Version:  v1.Version,
				Type:     v1.FrameData,
				Flags:    v1.FlagNone,
				StreamID: frame.StreamID,
				Payload:  responseData,
			}
			if err := connector.SendFrame(dataFrame); err != nil {
				logger.Error("Failed to send response", "error", err, "streamID", frame.StreamID)
			}

			// Send end stream
			endFrame := &v1.Frame{
				Version:  v1.Version,
				Type:     v1.FrameData,
				Flags:    v1.FlagEndStream,
				StreamID: frame.StreamID,
				Payload:  nil,
			}
			_ = connector.SendFrame(endFrame)

			// Close stream
			streamManager.CloseStream(frame.StreamID)
		}()

	case v1.FrameData:
		// Data frame - forward to stream
		stream, ok := streamManager.GetStream(frame.StreamID)
		if !ok {
			return client.ErrStreamNotFound
		}

		select {
		case stream.DataOut() <- frame.Payload:
		case <-stream.CloseCh():
			return client.ErrStreamNotFound
		}

		// Check EndStream flag
		if frame.IsEndStream() {
			streamManager.CloseStream(frame.StreamID)
		}

	case v1.FrameClose:
		// Close stream
		streamManager.CloseStream(frame.StreamID)

	default:
		logger.Warn("Unknown stream frame type", "type", frame.Type, "streamID", frame.StreamID)
	}

	return nil
}

// parseInt parses string to int
func parseInt(s string) (int, error) {
	return strconv.Atoi(s)
}
