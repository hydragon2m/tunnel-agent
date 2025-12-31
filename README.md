# Tunnel Agent

Tunnel Agent là client component của Go-tunnel system, kết nối đến Core Server và forward incoming requests từ Core đến local services.

## 📋 Tổng quan

Tunnel Agent là một lightweight client chạy trên local machine, tạo persistent connection đến Core Server và forward HTTP requests từ public internet đến local services. Agent hỗ trợ:

- **TLS Connection**: Secure connection đến Core Server
- **Stream Multiplexing**: Handle multiple concurrent requests qua single connection
- **Automatic Reconnection**: Smart reconnection với exponential backoff
- **Health Monitoring**: Built-in health checks và metrics
- **Structured Logging**: Production-ready logging với slog

## 🏗️ Architecture

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

### Components

1. **Connector**: TLS connection management với automatic reconnection
2. **Dispatcher**: Frame read loop và routing
3. **Stream Manager**: Stream lifecycle management
4. **Local Forwarder**: HTTP forwarding đến local services
5. **Authentication**: Auth handshake với Core
6. **Heartbeat**: Keepalive với Core

Xem [ARCHITECTURE.md](./ARCHITECTURE.md) để biết thêm chi tiết.

## 🚀 Installation

### Build from source

```bash
cd tunnel-agent
go build ./cmd/agent
```

### Run

```bash
./agent -server=localhost:8443 -token=your-token -local=http://localhost:3000
```

## ⚙️ Configuration

### Command-line Flags

#### Required

- `-token string`: Authentication token (required)

#### Server Configuration

- `-server string`: Core server address (default: "localhost:8443")
- `-tls`: Use TLS connection (default: true)
- `-skip-verify`: Skip TLS certificate verification (default: false)

#### Authentication

- `-agent-id string`: Agent ID (optional)
- `-version string`: Agent version (default: "1.0.0")

#### Local Service

- `-local string`: Local service URL (default: "http://localhost:3000")

#### Timeouts

- `-heartbeat duration`: Heartbeat interval (default: 10s)
- `-read-timeout duration`: Read timeout (default: 30s)
- `-request-timeout duration`: Request timeout (default: 30s)

#### Logging

- `-log-level string`: Log level: debug, info, warn, error (default: "info")
- `-log-json`: Use JSON logging format

#### Metrics

- `-metrics`: Enable metrics collection
- `-metrics-port int`: Metrics HTTP server port (default: 9091)

### Example Configuration

```bash
./agent \
  -server=core.example.com:8443 \
  -token=your-secret-token \
  -agent-id=agent-001 \
  -local=http://localhost:8080 \
  -tls=true \
  -heartbeat=10s \
  -read-timeout=30s \
  -request-timeout=30s \
  -log-level=info \
  -log-json \
  -metrics \
  -metrics-port=9091
```

## 📖 Usage

### Basic Usage

```bash
# Connect to Core Server
./agent -server=localhost:8443 -token=my-token -local=http://localhost:3000
```

### With TLS

```bash
# Use TLS (default)
./agent -server=core.example.com:8443 -token=my-token -local=http://localhost:3000 -tls

# Skip certificate verification (development only)
./agent -server=core.example.com:8443 -token=my-token -local=http://localhost:3000 -skip-verify
```

### With Logging

```bash
# Text format
./agent -server=localhost:8443 -token=my-token -log-level=debug

# JSON format (for log aggregation)
./agent -server=localhost:8443 -token=my-token -log-level=info -log-json
```

### With Metrics

```bash
# Enable metrics server
./agent -server=localhost:8443 -token=my-token -metrics -metrics-port=9091

# Access metrics
curl http://localhost:9091/metrics
curl http://localhost:9091/health
```

## 📊 Monitoring

### Metrics Endpoint

Khi enable metrics (`-metrics`), agent expose HTTP endpoints:

#### GET /metrics

Returns full metrics snapshot (JSON):

```json
{
  "connections": {
    "total": 10,
    "active": 1,
    "reconnections": 2,
    "reconnection_errors": 0
  },
  "streams": {
    "total": 150,
    "active": 5,
    "completed": 145,
    "failed": 0
  },
  "requests": {
    "total": 150,
    "success": 148,
    "failed": 2,
    "duration_us": 125000
  },
  "frames": {
    "received": 300,
    "sent": 300,
    "errors": 0
  },
  "heartbeat": {
    "sent": 100,
    "failed": 0
  },
  "local_service": {
    "requests_total": 150,
    "requests_error": 2,
    "duration_us": 120000
  },
  "timestamps": {
    "last_connection": "2024-01-15T10:30:00Z",
    "last_request": "2024-01-15T10:35:00Z",
    "last_heartbeat": "2024-01-15T10:35:05Z"
  },
  "health": {
    "status": "healthy"
  }
}
```

#### GET /health

Returns health status và checks:

```json
{
  "status": "healthy",
  "checks": {
    "connection": {
      "status": "healthy",
      "message": "Connected to server",
      "last_check": "2024-01-15T10:35:00Z"
    },
    "streams": {
      "status": "healthy",
      "message": "Streams active",
      "last_check": "2024-01-15T10:35:00Z"
    },
    "local_service": {
      "status": "healthy",
      "message": "Local service responding",
      "last_check": "2024-01-15T10:35:00Z"
    }
  }
}
```

### Health Status

- `healthy`: All checks passing
- `degraded`: Some checks failing (non-critical)
- `unhealthy`: Critical checks failing

## 🔍 Logging

### Log Levels

- `debug`: Detailed debugging information
- `info`: General informational messages
- `warn`: Warning messages
- `error`: Error messages

### Log Format

#### Text Format (default)

```
2024/01/15 10:30:00 INFO Starting Tunnel Agent version=1.0.0 agentID=agent-001
2024/01/15 10:30:01 INFO Connected to server address=localhost:8443
2024/01/15 10:30:02 INFO Authentication successful
```

#### JSON Format (`-log-json`)

```json
{"time":"2024-01-15T10:30:00Z","level":"INFO","msg":"Starting Tunnel Agent","version":"1.0.0","agentID":"agent-001"}
{"time":"2024-01-15T10:30:01Z","level":"INFO","msg":"Connected to server","address":"localhost:8443"}
{"time":"2024-01-15T10:30:02Z","level":"INFO","msg":"Authentication successful"}
```

## 🔄 Connection Flow

1. **Connect**: Agent connects to Core Server (TLS)
2. **Authenticate**: Send authentication frame với token
3. **Heartbeat**: Start periodic heartbeat
4. **Ready**: Agent ready to receive requests

### Reconnection

Agent tự động reconnect khi connection bị đứt:
- Exponential backoff: 1s → 2s → 4s → 8s → max 60s
- Tracks consecutive errors
- Aggressive backoff sau 5 consecutive errors

## 📡 Request Flow

1. **Core → Agent**: Core sends `FrameOpenStream` với HTTP request
2. **Agent**: Parse request và forward đến local service
3. **Local Service**: Process request và return response
4. **Agent → Core**: Agent sends response qua `FrameData`
5. **Close**: Agent sends `FrameData` với `FlagEndStream`

## 🛠️ Troubleshooting

### Connection Issues

**Problem**: Cannot connect to Core Server

```bash
# Check server address
./agent -server=core.example.com:8443 -token=my-token

# Check TLS
./agent -server=core.example.com:8443 -token=my-token -tls=false

# Skip certificate verification (dev only)
./agent -server=core.example.com:8443 -token=my-token -skip-verify
```

**Problem**: Authentication failed

- Verify token is correct
- Check token permissions
- Check Core Server logs

### Local Service Issues

**Problem**: Local service not responding

```bash
# Check local service URL
curl http://localhost:3000/health

# Verify agent can reach local service
./agent -local=http://localhost:3000 -log-level=debug
```

**Problem**: Timeout errors

- Increase `-request-timeout` value
- Check local service performance
- Check network latency

### Debugging

**Enable debug logging:**

```bash
./agent -log-level=debug -server=localhost:8443 -token=my-token
```

**Check metrics:**

```bash
# Enable metrics
./agent -metrics -metrics-port=9091

# Check health
curl http://localhost:9091/health

# Check metrics
curl http://localhost:9091/metrics
```

## 🔐 Security

### TLS

- Agent uses TLS để secure connection đến Core Server
- Certificate validation enabled by default
- Use `-skip-verify` chỉ trong development

### Authentication

- Token-based authentication
- Token được gửi trong authentication frame
- Token không được log (security best practice)

### Best Practices

1. **Never log tokens**: Tokens không được log
2. **Use TLS**: Always use TLS trong production
3. **Validate certificates**: Don't skip certificate verification
4. **Secure tokens**: Store tokens securely (env vars, secrets manager)

## 📚 Documentation

- [ARCHITECTURE.md](./ARCHITECTURE.md) - Architecture details
- [IMPROVEMENTS.md](./IMPROVEMENTS.md) - Recent improvements
- [STATUS.md](./STATUS.md) - Current status
- [REVIEW.md](./REVIEW.md) - Code review

## 🧪 Testing

### Manual Testing

```bash
# Start agent
./agent -server=localhost:8443 -token=test-token -local=http://localhost:3000 -log-level=debug

# In another terminal, check metrics
curl http://localhost:9091/health
```

### Integration Testing

1. Start Core Server
2. Start Agent
3. Send request đến Core Server public endpoint
4. Verify request forwarded đến local service
5. Check response returned correctly

## 🚀 Production Deployment

### Systemd Service

Create `/etc/systemd/system/tunnel-agent.service`:

```ini
[Unit]
Description=Tunnel Agent
After=network.target

[Service]
Type=simple
User=tunnel
ExecStart=/usr/local/bin/agent \
  -server=core.example.com:8443 \
  -token=${TOKEN} \
  -local=http://localhost:8080 \
  -log-level=info \
  -log-json \
  -metrics \
  -metrics-port=9091
Restart=always
RestartSec=5
Environment="TOKEN=your-token-here"

[Install]
WantedBy=multi-user.target
```

Enable và start:

```bash
sudo systemctl enable tunnel-agent
sudo systemctl start tunnel-agent
sudo systemctl status tunnel-agent
```

### Docker

```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o agent ./cmd/agent

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/agent .
CMD ["./agent", "-server=core.example.com:8443", "-token=${TOKEN}", "-local=http://localhost:8080"]
```

### Environment Variables

```bash
export TUNNEL_SERVER=core.example.com:8443
export TUNNEL_TOKEN=your-token
export TUNNEL_LOCAL=http://localhost:8080
export TUNNEL_LOG_LEVEL=info
export TUNNEL_METRICS=true
export TUNNEL_METRICS_PORT=9091

./agent
```

## 📈 Performance

### Benchmarks

- **Connection**: < 100ms
- **Request latency**: < 10ms overhead
- **Throughput**: 1000+ requests/second
- **Memory**: ~20MB baseline

### Optimization Tips

1. **Connection pooling**: Single connection cho tất cả requests
2. **Stream multiplexing**: Multiple streams qua single connection
3. **Efficient serialization**: Binary protocol
4. **Minimal overhead**: Direct forwarding

## 🤝 Contributing

1. Fork repository
2. Create feature branch
3. Make changes
4. Add tests
5. Submit pull request

## 📄 License

See LICENSE file for details.

## 🔗 Related

- [Tunnel Core](../tunnel-core/README.md) - Core Server
- [Tunnel Protocol](../tunnel-protocol/README.md) - Protocol specification

