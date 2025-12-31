package client

import "errors"

var (
	ErrNotConnected       = errors.New("not connected to server")
	ErrConnectionClosed    = errors.New("connection closed")
	ErrStreamNotFound      = errors.New("stream not found")
	ErrStreamAlreadyExists = errors.New("stream already exists")
	ErrInvalidFrame        = errors.New("invalid frame")
	ErrAuthFailed          = errors.New("authentication failed")
	ErrLocalServiceError   = errors.New("local service error")
	ErrAlreadyRunning      = errors.New("dispatcher already running")
)

