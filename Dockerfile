FROM golang:1.24-alpine AS builder

WORKDIR /build

# Copy tunnel-protocol first (dependency)
COPY tunnel-protocol/go.mod tunnel-protocol/go.sum* ./tunnel-protocol/

# Copy tunnel-agent go.mod files
COPY tunnel-agent/go.mod tunnel-agent/go.sum* ./tunnel-agent/

WORKDIR /build/tunnel-agent

# Download dependencies
RUN go mod download

# Copy source code
COPY tunnel-protocol/ ../tunnel-protocol/
COPY tunnel-agent/ .

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /agent ./cmd/agent

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata wget

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /agent .

# Expose metrics port
EXPOSE 9091

# Run
CMD ["./agent"]
