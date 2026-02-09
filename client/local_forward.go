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
	localServices map[string]string // subdomain -> localURL
	defaultURL    string
	httpClient    *http.Client
	timeout       time.Duration
}

// NewLocalForwarder tạo LocalForwarder mới
func NewLocalForwarder(defaultURL string, timeout time.Duration) *LocalForwarder {
	return &LocalForwarder{
		localServices: make(map[string]string),
		defaultURL:    defaultURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:       100,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: false,
			},
		},
		timeout: timeout,
	}
}

// AddService thêm mapping service mới
func (lf *LocalForwarder) AddService(subdomain, localURL string) {
	lf.localServices[subdomain] = localURL
}

// SetDefaultURL đặt default local URL
func (lf *LocalForwarder) SetDefaultURL(url string) {
	lf.defaultURL = url
}

// GetDefaultURL lấy default local URL
func (lf *LocalForwarder) GetDefaultURL() string {
	return lf.defaultURL
}

// GetSubdomains trả về danh sách các subdomain đã đăng ký
func (lf *LocalForwarder) GetSubdomains() []string {
	subs := make([]string, 0, len(lf.localServices))
	for sub := range lf.localServices {
		if sub != "" {
			subs = append(subs, sub)
		}
	}
	return subs
}

// ForwardRequest forward request từ Core đến local service
func (lf *LocalForwarder) ForwardRequest(ctx context.Context, stream *Stream, initialPayload []byte) error {
	startTime := time.Now()
	metrics.GetMetrics().IncrementLocalRequestsTotal()
	metrics.GetMetrics().IncrementRequestsTotal()

	// 1. Parse HTTP request headers from initial payload
	method, path, query, headers, initialBody, err := lf.parseRequest(initialPayload)
	if err != nil {
		metrics.GetMetrics().IncrementLocalRequestsError()
		metrics.GetMetrics().IncrementRequestsFailed()
		return fmt.Errorf("failed to parse request: %w", err)
	}

	// 2. Determine local URL based on Host header
	localBaseURL := lf.determineLocalURL(headers.Get("Host"))
	localURL := lf.buildLocalURL(localBaseURL, path, query)

	// 3. Create local HTTP request
	var bodyReader io.Reader
	contentLength := headers.Get("Content-Length")
	transferEncoding := headers.Get("Transfer-Encoding")

	if (contentLength != "" && contentLength != "0") || transferEncoding != "" {
		bodyReader = io.MultiReader(bytes.NewReader(initialBody), stream)
	} else if len(initialBody) > 0 {
		bodyReader = bytes.NewReader(initialBody)
	}

	// 4. Create local HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, method, localURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create local request: %w", err)
	}

	// Copy headers
	for key, values := range headers {
		if strings.ToLower(key) != "host" {
			for _, value := range values {
				httpReq.Header.Add(key, value)
			}
		}
	}

	// 5. Execute local request
	resp, err := lf.httpClient.Do(httpReq)
	if err != nil {
		metrics.GetMetrics().IncrementLocalRequestsError()
		return fmt.Errorf("local service request failed: %w", err)
	}
	defer resp.Body.Close()

	// 6. Write response line and headers back to the stream
	if err := lf.writeResponseHeader(stream, resp); err != nil {
		return fmt.Errorf("failed to write response headers: %w", err)
	}

	// 7. Stream response body back to the tunnel stream
	_, err = io.Copy(stream, resp.Body)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to stream response body: %w", err)
	}

	// Record metrics
	duration := time.Since(startTime)
	metrics.GetMetrics().RecordLocalRequestDuration(duration)
	metrics.GetMetrics().IncrementRequestsSuccess()
	metrics.GetMetrics().SetLastRequestTime(time.Now())

	return nil
}

// writeResponseHeader writes HTTP response line and headers to the stream
func (lf *LocalForwarder) writeResponseHeader(w io.Writer, resp *http.Response) error {
	var buf bytes.Buffer
	// Response line
	buf.WriteString(fmt.Sprintf("%s %s\r\n", resp.Proto, resp.Status))
	// Headers
	for key, values := range resp.Header {
		for _, value := range values {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}
	buf.WriteString("\r\n")
	_, err := w.Write(buf.Bytes())
	return err
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

// determineLocalURL quyết định local URL dựa trên host
func (lf *LocalForwarder) determineLocalURL(host string) string {
	if host == "" {
		return lf.defaultURL
	}

	// Extract subdomain (assuming host is sub.domain.com or sub.localhost)
	// We check if any of our keys match the start of the host
	for sub, url := range lf.localServices {
		if sub == "" {
			continue
		}
		if strings.HasPrefix(host, sub+".") || host == sub {
			logger.Debug("Matched local service", "host", host, "subdomain", sub, "url", url)
			return url
		}
	}

	logger.Debug("No mapping found for host, using default", "host", host, "default", lf.defaultURL)
	return lf.defaultURL
}

// buildLocalURL build local service URL
func (lf *LocalForwarder) buildLocalURL(baseURL, path, query string) string {
	url := baseURL
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
	buf.WriteString(fmt.Sprintf("%s %s\r\n", resp.Proto, resp.Status))

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
