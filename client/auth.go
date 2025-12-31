package client

import (
	"encoding/json"
	"fmt"
	"time"

	v1 "github.com/hydragon2m/tunnel-protocol/go/v1"
)

// Authenticator xử lý authentication với Core Server
type Authenticator struct {
	token      string
	agentID    string
	version    string
	capabilities []string
	metadata   map[string]string
	timeout    time.Duration
}

// AuthRequest là payload của FrameAuth
type AuthRequest struct {
	Token       string            `json:"token"`
	AgentID     string            `json:"agent_id,omitempty"`
	Version     string            `json:"version,omitempty"`
	Capabilities []string         `json:"capabilities,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// AuthResponse là payload của FrameAuth response
type AuthResponse struct {
	Success    bool              `json:"success"`
	AgentID    string            `json:"agent_id,omitempty"`
	ServerTime int64             `json:"server_time,omitempty"`
	Config     map[string]interface{} `json:"config,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// NewAuthenticator tạo Authenticator mới
func NewAuthenticator(token, agentID, version string, capabilities []string, metadata map[string]string) *Authenticator {
	return &Authenticator{
		token:        token,
		agentID:      agentID,
		version:      version,
		capabilities: capabilities,
		metadata:     metadata,
		timeout:      10 * time.Second,
	}
}

// CreateAuthFrame tạo FrameAuth để gửi đến Core
func (a *Authenticator) CreateAuthFrame() (*v1.Frame, error) {
	req := AuthRequest{
		Token:        a.token,
		AgentID:      a.agentID,
		Version:      a.version,
		Capabilities: a.capabilities,
		Metadata:     a.metadata,
	}
	
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	
	return &v1.Frame{
		Version:  v1.Version,
		Type:     v1.FrameAuth,
		Flags:    v1.FlagNone,
		StreamID: v1.StreamIDControl,
		Payload:  payload,
	}, nil
}

// HandleAuthResponse xử lý FrameAuth response từ Core
func (a *Authenticator) HandleAuthResponse(frame *v1.Frame) error {
	if frame.Type != v1.FrameAuth {
		return ErrInvalidFrame
	}
	
	if !frame.IsControlFrame() {
		return ErrInvalidFrame
	}
	
	if !frame.IsAck() {
		return ErrAuthFailed
	}
	
	var resp AuthResponse
	if err := json.Unmarshal(frame.Payload, &resp); err != nil {
		return err
	}
	
	if !resp.Success {
		return fmt.Errorf("auth failed: %s", resp.Error)
	}
	
	// Update agent ID if provided by server
	if resp.AgentID != "" {
		a.agentID = resp.AgentID
	}
	
	return nil
}

