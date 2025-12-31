package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hydragon2m/tunnel-agent/internal/logger"
	"github.com/hydragon2m/tunnel-agent/internal/metrics"
)

// LocalForwarder forward requests đến local services
type LocalForwarder struct {
	localURL    string
	httpClient  *http.Client
	timeout     time.Duration
}

// NewLocalForwarder tạo LocalForwarder mới
func NewLocalForwarder(localURL string, timeout time.Duration) *LocalForwarder {
	return &LocalForwarder{
		localURL: localURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  false,
			},
		},
		timeout: timeout,
	}
}

// ForwardRequest forward request từ Core đến local service
// Returns response data
func (lf *LocalForwarder) ForwardRequest(ctx context.Context, stream *Stream, requestData []byte) ([]byte, error) {
	startTime := time.Now()
	metrics.GetMetrics().IncrementLocalRequestsTotal()
	metrics.GetMetrics().IncrementRequestsTotal()
	
	// Parse HTTP request from payload
	method, path, query, headers, body, err := lf.parseRequest(requestData)
	if err != nil {
		metrics.GetMetrics().IncrementLocalRequestsError()
		metrics.GetMetrics().IncrementRequestsFailed()
		logger.Error("Failed to parse request", "error", err)
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}
	
	// Build local URL
	localURL := lf.buildLocalURL(path, query)
	
	logger.Debug("Forwarding request", "method", method, "url", localURL)
	
	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, localURL, bytes.NewReader(body))
	if err != nil {
		metrics.GetMetrics().IncrementLocalRequestsError()
		metrics.GetMetrics().IncrementRequestsFailed()
		logger.Error("Failed to create HTTP request", "error", err)
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	
	// Copy headers (except Host)
	for key, values := range headers {
		if strings.ToLower(key) != "host" {
			for _, value := range values {
				httpReq.Header.Add(key, value)
			}
		}
	}
	
	// Execute request
	resp, err := lf.httpClient.Do(httpReq)
	if err != nil {
		metrics.GetMetrics().IncrementLocalRequestsError()
		metrics.GetMetrics().IncrementRequestsFailed()
		logger.Error("Local service request failed", "error", err, "url", localURL)
		return nil, fmt.Errorf("local service error: %w", err)
	}
	defer resp.Body.Close()
	
	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.GetMetrics().IncrementLocalRequestsError()
		metrics.GetMetrics().IncrementRequestsFailed()
		logger.Error("Failed to read response body", "error", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	
	// Build response payload
	responseData := lf.buildResponse(resp, respBody)
	
	// Record metrics
	duration := time.Since(startTime)
	metrics.GetMetrics().RecordLocalRequestDuration(duration)
	metrics.GetMetrics().RecordRequestDuration(duration)
	metrics.GetMetrics().IncrementRequestsSuccess()
	metrics.GetMetrics().SetLastRequestTime(time.Now())
	
	logger.Debug("Request completed", "method", method, "url", localURL, "status", resp.StatusCode, "duration", duration)
	
	return responseData, nil
}

// parseRequest parse HTTP request từ payload
// Returns: method, path, query, headers, body, error
func (lf *LocalForwarder) parseRequest(data []byte) (string, string, string, http.Header, []byte, error) {
	// Parse HTTP request line và headers
	// Format: "METHOD PATH HTTP/1.1\r\nHeaders\r\n\r\nBody"
	
	parts := bytes.SplitN(data, []byte("\r\n\r\n"), 2)
	if len(parts) < 1 {
		return "", "", "", nil, nil, fmt.Errorf("invalid request format")
	}
	
	headerPart := parts[0]
	var body []byte
	if len(parts) > 1 {
		body = parts[1]
	}
	
	// Parse request line
	lines := bytes.Split(headerPart, []byte("\r\n"))
	if len(lines) < 1 {
		return "", "", "", nil, nil, fmt.Errorf("invalid request line")
	}
	
	requestLine := string(lines[0])
	requestParts := strings.Split(requestLine, " ")
	if len(requestParts) < 3 {
		return "", "", "", nil, nil, fmt.Errorf("invalid request line format")
	}
	
	method := requestParts[0]
	pathWithQuery := requestParts[1]
	
	// Split path and query
	path := pathWithQuery
	query := ""
	if idx := strings.Index(pathWithQuery, "?"); idx != -1 {
		path = pathWithQuery[:idx]
		query = pathWithQuery[idx+1:]
	}
	
	// Parse headers
	headers := make(http.Header)
	for i := 1; i < len(lines); i++ {
		line := string(lines[i])
		colonIndex := strings.Index(line, ":")
		if colonIndex == -1 {
			continue
		}
		
		key := strings.TrimSpace(line[:colonIndex])
		value := strings.TrimSpace(line[colonIndex+1:])
		headers.Add(key, value)
	}
	
	return method, path, query, headers, body, nil
}

// buildLocalURL build local service URL
func (lf *LocalForwarder) buildLocalURL(path, query string) string {
	url := lf.localURL
	if !strings.HasSuffix(url, "/") && !strings.HasPrefix(path, "/") {
		url += "/"
	}
	url += strings.TrimPrefix(path, "/")
	
	if query != "" {
		url += "?" + query
	}
	
	return url
}

// buildResponse build HTTP response payload
func (lf *LocalForwarder) buildResponse(resp *http.Response, body []byte) []byte {
	var buf bytes.Buffer
	
	// Response line
	buf.WriteString(fmt.Sprintf("%s %d %s\r\n", resp.Proto, resp.StatusCode, resp.Status))
	
	// Headers
	for key, values := range resp.Header {
		for _, value := range values {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}
	
	buf.WriteString("\r\n")
	
	// Body
	if len(body) > 0 {
		buf.Write(body)
	}
	
	return buf.Bytes()
}

