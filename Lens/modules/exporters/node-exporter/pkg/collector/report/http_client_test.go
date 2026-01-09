// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package report

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestNewHTTPReporter(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		nodeName string
		nodeIP   string
	}{
		{
			name:     "Valid configuration",
			baseURL:  "http://localhost:8080",
			nodeName: "node-1",
			nodeIP:   "192.168.1.100",
		},
		{
			name:     "HTTPS URL",
			baseURL:  "https://api.example.com",
			nodeName: "prod-node",
			nodeIP:   "10.0.0.1",
		},
		{
			name:     "Empty node name",
			baseURL:  "http://localhost:8080",
			nodeName: "",
			nodeIP:   "192.168.1.100",
		},
		{
			name:     "IPv6 address",
			baseURL:  "http://localhost:8080",
			nodeName: "node-1",
			nodeIP:   "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := NewHTTPReporter(tt.baseURL, tt.nodeName, tt.nodeIP)

			assert.NotNil(t, reporter)
			assert.Equal(t, tt.baseURL, reporter.baseURL)
			assert.Equal(t, tt.nodeName, reporter.nodeName)
			assert.Equal(t, tt.nodeIP, reporter.nodeIP)
			assert.NotNil(t, reporter.httpClient)
			assert.NotNil(t, reporter.eventBuffer)
			assert.NotNil(t, reporter.stopChan)
			assert.Equal(t, 120*time.Second, reporter.httpClient.Timeout)
			assert.True(t, reporter.batchEnabled)
			assert.Equal(t, 10, reporter.batchSize)
			assert.Equal(t, 5*time.Second, reporter.batchTimeout)
		})
	}
}

func TestHTTPReporter_SetBatchConfig(t *testing.T) {
	reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

	t.Run("Enable batch mode", func(t *testing.T) {
		reporter.SetBatchConfig(true, 20, 10*time.Second)

		assert.True(t, reporter.batchEnabled)
		assert.Equal(t, 20, reporter.batchSize)
		assert.Equal(t, 10*time.Second, reporter.batchTimeout)
	})

	t.Run("Disable batch mode", func(t *testing.T) {
		reporter.SetBatchConfig(false, 5, 1*time.Second)

		assert.False(t, reporter.batchEnabled)
		assert.Equal(t, 5, reporter.batchSize)
		assert.Equal(t, 1*time.Second, reporter.batchTimeout)
	})

	t.Run("Increase buffer size", func(t *testing.T) {
		initialCap := cap(reporter.eventBuffer)
		reporter.SetBatchConfig(true, 100, 5*time.Second)

		assert.Equal(t, 100, reporter.batchSize)
		assert.GreaterOrEqual(t, cap(reporter.eventBuffer), initialCap)
	})

	t.Run("Zero batch size", func(t *testing.T) {
		reporter.SetBatchConfig(true, 0, 1*time.Second)

		assert.Equal(t, 0, reporter.batchSize)
	})

	t.Run("Negative timeout", func(t *testing.T) {
		reporter.SetBatchConfig(true, 10, -1*time.Second)

		assert.Equal(t, -1*time.Second, reporter.batchTimeout)
	})
}

func TestHTTPReporter_AddToBuffer(t *testing.T) {
	reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")
	reporter.SetBatchConfig(true, 3, 10*time.Second)

	t.Run("Add single event to buffer", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0] // Clear buffer

		event := ContainerEventRequest{
			Type:        "created",
			Source:      "k8s",
			Node:        "node-1",
			ContainerID: "container-123",
			Data:        map[string]interface{}{"test": "data"},
		}

		err := reporter.addToBuffer(event)
		assert.NoError(t, err)
		assert.Equal(t, 1, len(reporter.eventBuffer))
	})

	t.Run("Buffer does not flush when not full", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0] // Clear buffer
		reporter.SetBatchConfig(true, 5, 10*time.Second)

		for i := 0; i < 3; i++ {
			event := ContainerEventRequest{
				ContainerID: "container-" + string(rune('0'+i)),
			}
			err := reporter.addToBuffer(event)
			assert.NoError(t, err)
		}

		assert.Equal(t, 3, len(reporter.eventBuffer))
	})

	t.Run("Buffer content validation", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0] // Clear buffer

		event := ContainerEventRequest{
			Type:        "running",
			Source:      "docker",
			Node:        "test-node",
			ContainerID: "abc123",
			Data: map[string]interface{}{
				"pod_name": "test-pod",
				"status":   "running",
			},
		}

		err := reporter.addToBuffer(event)
		assert.NoError(t, err)

		assert.Equal(t, 1, len(reporter.eventBuffer))
		bufferedEvent := reporter.eventBuffer[0]
		assert.Equal(t, "running", bufferedEvent.Type)
		assert.Equal(t, "docker", bufferedEvent.Source)
		assert.Equal(t, "test-node", bufferedEvent.Node)
		assert.Equal(t, "abc123", bufferedEvent.ContainerID)
		assert.Equal(t, "test-pod", bufferedEvent.Data["pod_name"])
	})
}

func TestHTTPReporter_ConcurrentAccess(t *testing.T) {
	reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")
	reporter.SetBatchConfig(true, 1000, 10*time.Second)

	t.Run("Concurrent add to buffer", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0] // Clear buffer

		var wg sync.WaitGroup
		numGoroutines := 100
		eventsPerGoroutine := 10

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for j := 0; j < eventsPerGoroutine; j++ {
					event := ContainerEventRequest{
						ContainerID: "container-" + string(rune('0'+id%10)),
						Type:        "test",
					}
					reporter.addToBuffer(event)
				}
			}(i)
		}

		wg.Wait()

		// Buffer should contain all events (unless flush was triggered)
		assert.LessOrEqual(t, len(reporter.eventBuffer), numGoroutines*eventsPerGoroutine)
	})

	t.Run("Concurrent SetBatchConfig and addToBuffer", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0]

		var wg sync.WaitGroup

		// Goroutine 1: Add events
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				event := ContainerEventRequest{
					ContainerID: "test",
				}
				reporter.addToBuffer(event)
				time.Sleep(1 * time.Millisecond)
			}
		}()

		// Goroutine 2: Change config
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				reporter.SetBatchConfig(true, 20+i, 5*time.Second)
				time.Sleep(5 * time.Millisecond)
			}
		}()

		wg.Wait()

		// Should not panic and complete successfully
		assert.True(t, true)
	})
}

func TestHTTPReporter_FlushLocked(t *testing.T) {
	reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

	t.Run("Flush empty buffer", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0]

		// flushLocked should handle empty buffer gracefully
		// Note: This will try to send HTTP request which will fail,
		// but we're testing the buffer handling logic
		err := reporter.flushLocked()
		// Empty buffer should return nil without attempting to send
		assert.NoError(t, err)
	})

	t.Run("Buffer cleared after flush attempt", func(t *testing.T) {
		reporter.eventBuffer = reporter.eventBuffer[:0]

		// Add some events
		for i := 0; i < 5; i++ {
			reporter.eventBuffer = append(reporter.eventBuffer, ContainerEventRequest{
				ContainerID: "test-" + string(rune('0'+i)),
			})
		}

		assert.Equal(t, 5, len(reporter.eventBuffer))

		// Flush will fail due to network, but buffer should still be cleared
		reporter.flushLocked()

		// Buffer should be empty after flush attempt
		assert.Equal(t, 0, len(reporter.eventBuffer))
	})
}

func TestContainerEventRequest_Structure(t *testing.T) {
	t.Run("Create event request", func(t *testing.T) {
		event := ContainerEventRequest{
			Type:        "snapshot",
			Source:      "k8s",
			Node:        "worker-1",
			ContainerID: "abc123xyz",
			Data: map[string]interface{}{
				"pod_name":      "my-pod",
				"pod_namespace": "default",
				"gpu_count":     4,
			},
		}

		assert.Equal(t, "snapshot", event.Type)
		assert.Equal(t, "k8s", event.Source)
		assert.Equal(t, "worker-1", event.Node)
		assert.Equal(t, "abc123xyz", event.ContainerID)
		assert.Equal(t, "my-pod", event.Data["pod_name"])
		assert.Equal(t, "default", event.Data["pod_namespace"])
		assert.Equal(t, 4, event.Data["gpu_count"])
	})

	t.Run("Empty event request", func(t *testing.T) {
		event := ContainerEventRequest{}

		assert.Empty(t, event.Type)
		assert.Empty(t, event.Source)
		assert.Empty(t, event.Node)
		assert.Empty(t, event.ContainerID)
		assert.Nil(t, event.Data)
	})
}

func TestBatchContainerEventsRequest_Structure(t *testing.T) {
	t.Run("Create batch request", func(t *testing.T) {
		events := []ContainerEventRequest{
			{
				Type:        "created",
				ContainerID: "c1",
			},
			{
				Type:        "started",
				ContainerID: "c2",
			},
		}

		batch := BatchContainerEventsRequest{
			Events: events,
		}

		assert.Equal(t, 2, len(batch.Events))
		assert.Equal(t, "created", batch.Events[0].Type)
		assert.Equal(t, "started", batch.Events[1].Type)
	})

	t.Run("Empty batch request", func(t *testing.T) {
		batch := BatchContainerEventsRequest{}

		assert.Nil(t, batch.Events)
	})
}

func TestHTTPReporter_StartStop(t *testing.T) {
	t.Run("Start and stop reporter", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

		// Start should not block
		reporter.Start()

		// Give it a moment to start
		time.Sleep(10 * time.Millisecond)

		// Stop should complete quickly
		reporter.Stop()

		// Should complete without hanging
		assert.True(t, true)
	})

	t.Run("Multiple start calls", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

		reporter.Start()
		reporter.Start() // Second start should not cause issues

		time.Sleep(10 * time.Millisecond)

		reporter.Stop()
		assert.True(t, true)
	})

	t.Run("Stop without start", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

		// Stop without start should not panic
		reporter.Stop()
		assert.True(t, true)
	})
}

func TestHTTPReporter_EdgeCases(t *testing.T) {
	t.Run("Reporter with empty baseURL", func(t *testing.T) {
		reporter := NewHTTPReporter("", "node", "127.0.0.1")

		assert.NotNil(t, reporter)
		assert.Equal(t, "", reporter.baseURL)
	})

	t.Run("Reporter with very long URL", func(t *testing.T) {
		longURL := "http://very-long-domain-name-that-exceeds-normal-length"
		for i := 0; i < 10; i++ {
			longURL += ".subdomain"
		}
		longURL += ".com/api/v1/very/long/path"

		reporter := NewHTTPReporter(longURL, "node", "127.0.0.1")

		assert.NotNil(t, reporter)
		assert.Equal(t, longURL, reporter.baseURL)
	})

	t.Run("Add large number of events", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")
		reporter.SetBatchConfig(true, 10000, 10*time.Second)
		reporter.eventBuffer = reporter.eventBuffer[:0]

		for i := 0; i < 1000; i++ {
			event := ContainerEventRequest{
				ContainerID: "container-" + string(rune('0'+i%10)),
			}
			reporter.addToBuffer(event)
		}

		assert.LessOrEqual(t, len(reporter.eventBuffer), 1000)
	})
}

func TestHTTPReporter_TimeoutConfiguration(t *testing.T) {
	t.Run("Default timeout is 120 seconds", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

		assert.Equal(t, 120*time.Second, reporter.httpClient.Timeout)
	})
}

func TestHTTPReporter_BufferCapacity(t *testing.T) {
	t.Run("Initial buffer capacity", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

		assert.Equal(t, 10, cap(reporter.eventBuffer))
		assert.Equal(t, 0, len(reporter.eventBuffer))
	})

	t.Run("Buffer capacity increases with config", func(t *testing.T) {
		reporter := NewHTTPReporter("http://localhost", "node", "127.0.0.1")

		reporter.SetBatchConfig(true, 50, 5*time.Second)

		assert.GreaterOrEqual(t, cap(reporter.eventBuffer), 50)
	})
}

func TestGlobalFunctions(t *testing.T) {
	// Reset global reporter for clean state
	globalHTTPReporter = nil

	t.Run("InitHTTP creates global reporter", func(t *testing.T) {
		err := InitHTTP("http://localhost:8080", "test-node", "192.168.1.1")
		assert.NoError(t, err)
		assert.NotNil(t, globalHTTPReporter)

		// Clean up
		Shutdown()
		globalHTTPReporter = nil
	})

	t.Run("ReportContainer requires initialization", func(t *testing.T) {
		globalHTTPReporter = nil

		container := &model.Container{}
		container.Id = "test-container"

		err := ReportContainer(context.Background(), container, "test")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("FlushEvents requires initialization", func(t *testing.T) {
		globalHTTPReporter = nil

		err := FlushEvents()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})

	t.Run("SetBatchConfig on uninitialized reporter", func(t *testing.T) {
		globalHTTPReporter = nil

		// Should not panic
		SetBatchConfig(true, 10, 5*time.Second)
		assert.True(t, true)
	})

	t.Run("Shutdown on uninitialized reporter", func(t *testing.T) {
		globalHTTPReporter = nil

		// Should not panic
		Shutdown()
		assert.True(t, true)
	})
}
