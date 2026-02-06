package client

import (
	"sync"
	"testing"
	"time"
)

func TestStreamManager_CreateStream(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	stream, err := sm.CreateStream(1)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	if stream.ID != 1 {
		t.Errorf("Expected stream ID 1, got %d", stream.ID)
	}

	if stream.State != StreamStateInit {
		t.Errorf("Expected state Init, got %v", stream.State)
	}

	retrieved, ok := sm.GetStream(1)
	if !ok {
		t.Error("Stream should be registered")
	}
	if retrieved.ID != stream.ID {
		t.Error("Retrieved stream should match created stream")
	}
}

func TestStreamManager_CreateDuplicateStream(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	_, err := sm.CreateStream(1)
	if err != nil {
		t.Fatalf("Failed to create first stream: %v", err)
	}

	_, err = sm.CreateStream(1)
	if err != ErrStreamAlreadyExists {
		t.Errorf("Expected ErrStreamAlreadyExists, got %v", err)
	}
}

func TestStreamManager_GetStream_NotFound(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	_, ok := sm.GetStream(999)
	if ok {
		t.Error("Should not find non-existent stream")
	}
}

func TestStreamManager_CloseStream(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	stream, err := sm.CreateStream(1)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	err = sm.CloseStream(1)
	if err != nil {
		t.Errorf("Failed to close stream: %v", err)
	}

	_, ok := sm.GetStream(1)
	if ok {
		t.Error("Stream should be removed after close")
	}

	if stream.State != StreamStateClosed {
		t.Errorf("Expected state Closed, got %v", stream.State)
	}
}

func TestStreamManager_CloseNonExistentStream(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	err := sm.CloseStream(999)
	if err != ErrStreamNotFound {
		t.Errorf("Expected ErrStreamNotFound, got %v", err)
	}
}

func TestStreamManager_ConcurrentStreamCreation(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	var wg sync.WaitGroup
	numStreams := 100

	for i := 0; i < numStreams; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()
			_, err := sm.CreateStream(id)
			if err != nil {
				t.Errorf("Failed to create stream %d: %v", id, err)
			}
		}(uint32(i))
	}

	wg.Wait()

	for i := 0; i < numStreams; i++ {
		_, ok := sm.GetStream(uint32(i))
		if !ok {
			t.Errorf("Stream %d should exist", i)
		}
	}
}

func TestStreamManager_Callbacks(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	var createdID uint32
	var closedID uint32
	var createdCalled bool
	var closedCalled bool

	sm.SetOnStreamCreated(func(streamID uint32) {
		createdCalled = true
		createdID = streamID
	})

	sm.SetOnStreamClosed(func(streamID uint32) {
		closedCalled = true
		closedID = streamID
	})

	_, err := sm.CreateStream(42)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	if !createdCalled {
		t.Error("OnStreamCreated callback should be called")
	}
	if createdID != 42 {
		t.Errorf("Expected created ID 42, got %d", createdID)
	}

	err = sm.CloseStream(42)
	if err != nil {
		t.Fatalf("Failed to close stream: %v", err)
	}

	if !closedCalled {
		t.Error("OnStreamClosed callback should be called")
	}
	if closedID != 42 {
		t.Errorf("Expected closed ID 42, got %d", closedID)
	}
}

func TestStream_Metadata(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	stream, err := sm.CreateStream(1)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	stream.SetMetadata("key1", "value1")
	stream.SetMetadata("key2", "value2")

	val1, ok := stream.GetMetadata("key1")
	if !ok || val1 != "value1" {
		t.Error("Expected to get metadata key1=value1")
	}

	val2, ok := stream.GetMetadata("key2")
	if !ok || val2 != "value2" {
		t.Error("Expected to get metadata key2=value2")
	}

	_, ok = stream.GetMetadata("nonexistent")
	if ok {
		t.Error("Should not find non-existent metadata key")
	}
}

func TestStream_StateTransitions(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	stream, err := sm.CreateStream(1)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	if stream.GetState() != StreamStateInit {
		t.Errorf("Expected initial state Init, got %v", stream.GetState())
	}

	stream.setState(StreamStateOpen)
	if stream.GetState() != StreamStateOpen {
		t.Errorf("Expected state Open, got %v", stream.GetState())
	}

	stream.setState(StreamStateData)
	if stream.GetState() != StreamStateData {
		t.Errorf("Expected state Data, got %v", stream.GetState())
	}

	stream.setState(StreamStateClosed)
	if stream.GetState() != StreamStateClosed {
		t.Errorf("Expected state Closed, got %v", stream.GetState())
	}
}

func TestStream_Read(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	stream, err := sm.CreateStream(1)
	if err != nil {
		t.Fatalf("Failed to create stream: %v", err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		stream.dataOut <- []byte("Test Data")
	}()

	buf := make([]byte, 100)
	n, err := stream.Read(buf)
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if n != 9 {
		t.Errorf("Expected to read 9 bytes, read %d", n)
	}
	if string(buf[:n]) != "Test Data" {
		t.Errorf("Expected 'Test Data', got '%s'", buf[:n])
	}
}

func TestStreamManager_ConcurrentOperations(t *testing.T) {
	sm := &StreamManager{
		streams: make(map[uint32]*Stream),
	}

	var wg sync.WaitGroup
	numOps := 50

	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(id uint32) {
			defer wg.Done()

			_, err := sm.CreateStream(id)
			if err != nil {
				t.Errorf("Create failed for stream %d: %v", id, err)
				return
			}

			time.Sleep(time.Millisecond)

			err = sm.CloseStream(id)
			if err != nil {
				t.Errorf("Close failed for stream %d: %v", id, err)
			}
		}(uint32(i))
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Concurrent operations timed out")
	}
}
