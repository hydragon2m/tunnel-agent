# Tunnel Agent Architecture

## Tổng quan

Tunnel Agent là client kết nối đến Core Server và forward incoming requests từ Core đến local services.

## Kiến trúc tổng thể

```
┌─────────────┐         ┌──────────────┐         ┌─────────────┐
│   Core      │────────▶│    Agent     │────────▶│   Local     │
│   Server    │ Protocol│   (Client)   │  HTTP   │   Service   │
└─────────────┘   v1    └──────────────┘         └─────────────┘
                              │
                    ┌─────────┴─────────┐
                    │                   │
              ┌─────▼─────┐      ┌──────▼──────┐
              │  Stream   │      │   Local     │
              │  Manager  │      │  Forwarder  │
              └──────────┘      └─────────────┘
```

## Components

### 1. Connector (`client/connector.go`)
**Trách nhiệm:**
- Kết nối TLS tới Core Server
- Quản lý connection lifecycle
- Reconnection logic
- Connection state

**Key Features:**
- TLS connection với certificate validation
- Automatic reconnection với exponential backoff
- Connection health monitoring
- Graceful shutdown

### 2. Frame Handler (`client/dispatcher.go`)
**Trách nhiệm:**
- Frame read loop
- Frame decoding
- Frame routing (control vs data streams)
- Error handling

**Key Features:**
- Continuous frame reading
- Frame type routing
- Protocol error handling
- Timeout management

### 3. Stream Manager (`client/stream.go`)
**Trách nhiệm:**
- Stream lifecycle management
- Stream state machine
- Stream multiplexing
- Stream cleanup

**Key Features:**
- Stream registry
- State transitions (INIT → OPEN → DATA → CLOSED)
- Stream metadata
- Cleanup on close

### 4. Local Forwarder (`client/local_forward.go`)
**Trách nhiệm:**
- Forward requests đến local services
- HTTP client cho local services
- Response forwarding
- Error handling

**Key Features:**
- HTTP/1.1 client
- Request parsing từ frames
- Response serialization
- Timeout handling

### 5. Authentication (`client/auth.go`)
**Trách nhiệm:**
- Auth handshake với Core
- Token management
- Auth state

**Key Features:**
- FrameAuth sending
- Auth response handling
- Token validation
- Retry logic

### 6. Heartbeat (`client/heartbeat.go`)
**Trách nhiệm:**
- Keepalive với Core
- Connection health
- Timeout detection

**Key Features:**
- Periodic FrameHeartbeat
- Heartbeat interval config
- Connection health monitoring

## Data Flow

### 1. Connection Flow
```
Agent → Connect TLS to Core
      → Send FrameAuth (StreamID=0, token)
      → Receive FrameAuth (ACK, StreamID=0)
      → Connection established
      → Start heartbeat loop
```

### 2. Incoming Request Flow
```
Core → Send FrameOpenStream (StreamID=N)
     → Agent: receive FrameOpenStream
     → Agent: parse request payload
     → Agent: forward to local service (HTTP)
     → Local service: process request
     → Agent: receive response
     → Agent: send FrameData (StreamID=N, response data)
     → Agent: send FrameData (StreamID=N, FlagEndStream)
     → Stream closed
```

### 3. Stream Lifecycle
```
INIT → OPEN → DATA* → CLOSED
  │      │      │        │
  │      │      │        └─ Cleanup
  │      │      └─ Multiple DATA frames
  │      └─ FrameOpenStream received
  └─ New stream request
```

## Concurrency Model

- **Main goroutine**: Connection management, reconnection
- **Frame reader goroutine**: Continuous frame reading
- **Per-stream goroutines**: 2 goroutines per stream (read/write)
- **Heartbeat goroutine**: Periodic heartbeat sending
- **Local forwarder goroutines**: HTTP requests to local services

## Error Handling

- **Connection errors**: Reconnect với exponential backoff
- **Stream errors**: Send FrameError, close stream
- **Protocol errors**: Log, close connection, reconnect
- **Local service errors**: Forward error to Core

## Reconnection Strategy

1. **Exponential backoff**: 1s, 2s, 4s, 8s, max 60s
2. **Max retries**: Configurable (default: unlimited)
3. **Connection state**: Track connection state
4. **Graceful shutdown**: Close all streams before reconnect

## Local Service Forwarding

- **HTTP/1.1**: Forward HTTP requests to local services
- **Request parsing**: Parse HTTP from FrameOpenStream payload
- **Response serialization**: Serialize HTTP response to FrameData
- **Timeout**: Configurable timeout per request
- **Error handling**: Forward errors to Core

