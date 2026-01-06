package pyspy_task_dispatcher

import (
	"testing"
)

func TestNewPySpyTaskDispatcherJob(t *testing.T) {
	job := NewPySpyTaskDispatcherJob()
	if job == nil {
		t.Fatal("NewPySpyTaskDispatcherJob returned nil")
	}

	if job.facade == nil {
		t.Error("facade is nil")
	}

	if job.instanceID == "" {
		t.Error("instanceID is empty")
	}

	if job.client == nil {
		t.Error("client is nil")
	}
}

func TestSchedule(t *testing.T) {
	job := NewPySpyTaskDispatcherJob()
	schedule := job.Schedule()

	if schedule != JobSchedule {
		t.Errorf("Expected schedule %s, got %s", JobSchedule, schedule)
	}
}

func TestGenerateInstanceID(t *testing.T) {
	id1 := generateInstanceID()
	id2 := generateInstanceID()

	if id1 == "" {
		t.Error("generateInstanceID returned empty string")
	}

	if id1 == id2 {
		t.Error("generateInstanceID should return unique IDs")
	}

	// Check prefix
	if len(id1) < 16 {
		t.Error("instanceID should have proper length")
	}
}

func TestNewNodeExporterClient(t *testing.T) {
	client := NewNodeExporterClient()
	if client == nil {
		t.Fatal("NewNodeExporterClient returned nil")
	}

	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestNewNodeExporterResolver(t *testing.T) {
	resolver := NewNodeExporterResolver(nil)
	if resolver == nil {
		t.Fatal("NewNodeExporterResolver returned nil")
	}

	if resolver.port != DefaultNodeExporterPort {
		t.Errorf("Expected port %d, got %d", DefaultNodeExporterPort, resolver.port)
	}
}

func TestGetNodeExporterAddress_NoClient(t *testing.T) {
	resolver := NewNodeExporterResolver(nil)

	_, err := resolver.GetNodeExporterAddress(nil, "test-node")
	if err == nil {
		t.Error("Expected error when k8s client is nil")
	}
}

func TestGetAllNodeExporterAddresses_NoClient(t *testing.T) {
	resolver := NewNodeExporterResolver(nil)

	_, err := resolver.GetAllNodeExporterAddresses(nil)
	if err == nil {
		t.Error("Expected error when k8s client is nil")
	}
}

