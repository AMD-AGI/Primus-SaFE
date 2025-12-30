// Package e2e provides end-to-end tests for AI Gateway Phase 1-5
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
)

// Test configuration from environment
var (
	dbDSN = getEnv("TEST_DB_DSN", "host=localhost port=5432 dbname=lens user=lens password=lens sslmode=disable")
)

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

// TestAITopicsEnvelope tests the topic envelope structures (Phase 1)
func TestAITopicsEnvelope(t *testing.T) {
	t.Run("RequestEnvelope", func(t *testing.T) {
		req := &aitopics.Request{
			RequestID: "test-123",
			Topic:     aitopics.TopicAlertAdvisorAggregateWorkloads,
			Version:   "v1",
			Timestamp: time.Now(),
			Context: aitopics.RequestContext{
				ClusterID: "cluster-1",
				TenantID:  "tenant-1",
			},
			Payload: json.RawMessage(`{"workloads": []}`),
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Failed to marshal request: %v", err)
		}

		var parsed aitopics.Request
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal request: %v", err)
		}

		if parsed.Topic != req.Topic {
			t.Errorf("Topic mismatch: got %s, want %s", parsed.Topic, req.Topic)
		}

		if parsed.Context.ClusterID != req.Context.ClusterID {
			t.Errorf("ClusterID mismatch: got %s, want %s", parsed.Context.ClusterID, req.Context.ClusterID)
		}
	})

	t.Run("ResponseEnvelope", func(t *testing.T) {
		resp := &aitopics.Response{
			RequestID: "test-123",
			Status:    "success",
			Code:      0,
			Message:   "OK",
			Timestamp: time.Now(),
			Payload:   json.RawMessage(`{"result": "test"}`),
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Failed to marshal response: %v", err)
		}

		var parsed aitopics.Response
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if parsed.Status != "success" {
			t.Errorf("Status mismatch: got %s, want success", parsed.Status)
		}
	})
}

// TestAITopicSchemas tests topic-specific payload schemas (Phase 1)
func TestAITopicSchemas(t *testing.T) {
	t.Run("AggregateWorkloadsInput", func(t *testing.T) {
		input := &aitopics.AggregateWorkloadsInput{
			Workloads: []aitopics.WorkloadInfo{
				{
					UID:       "uid-1",
					Name:      "postgres-primary",
					Namespace: "default",
					Kind:      "Deployment",
					Labels:    map[string]string{"app": "postgres"},
					Images:    []string{"postgres:15"},
				},
			},
			Options: &aitopics.AggregateOptions{
				MaxGroups:     10,
				MinConfidence: 0.8,
			},
		}

		data, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var parsed aitopics.AggregateWorkloadsInput
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if len(parsed.Workloads) != 1 {
			t.Errorf("Workloads count mismatch: got %d, want 1", len(parsed.Workloads))
		}
	})

	t.Run("AggregateWorkloadsOutput", func(t *testing.T) {
		output := &aitopics.AggregateWorkloadsOutput{
			Groups: []aitopics.ComponentGroup{
				{
					GroupID:           "group-1",
					Name:              "PostgreSQL Cluster",
					ComponentType:     "postgresql",
					Category:          "database",
					Members:           []string{"uid-1", "uid-2"},
					AggregationReason: "Same image",
					Confidence:        0.95,
				},
			},
			Ungrouped: []string{},
			Stats: aitopics.AggregateStats{
				TotalWorkloads:   2,
				GroupedWorkloads: 2,
				TotalGroups:      1,
			},
		}

		data, err := json.Marshal(output)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		if len(data) == 0 {
			t.Error("Marshaled data is empty")
		}
	})
}

// TestMemoryRegistry tests the in-memory registry (Phase 2)
func TestMemoryRegistry(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	ctx := context.Background()

	t.Run("RegisterAndGet", func(t *testing.T) {
		agent := &airegistry.AgentRegistration{
			Name:     "test-agent",
			Endpoint: "http://localhost:8002",
			Topics:   []string{"test.topic.*"},
			Status:   airegistry.AgentStatusHealthy,
		}

		err := registry.Register(ctx, agent)
		if err != nil {
			t.Fatalf("Failed to register: %v", err)
		}

		retrieved, err := registry.Get(ctx, "test-agent")
		if err != nil {
			t.Fatalf("Failed to get: %v", err)
		}

		if retrieved.Endpoint != agent.Endpoint {
			t.Errorf("Endpoint mismatch: got %s, want %s", retrieved.Endpoint, agent.Endpoint)
		}
	})

	t.Run("ListAgents", func(t *testing.T) {
		agents, err := registry.List(ctx)
		if err != nil {
			t.Fatalf("Failed to list: %v", err)
		}

		if len(agents) == 0 {
			t.Error("Expected at least one agent")
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		err := registry.UpdateStatus(ctx, "test-agent", airegistry.AgentStatusUnhealthy, 1)
		if err != nil {
			t.Fatalf("Failed to update status: %v", err)
		}

		agent, _ := registry.Get(ctx, "test-agent")
		if agent.Status != airegistry.AgentStatusUnhealthy {
			t.Errorf("Status not updated: got %s, want unhealthy", agent.Status)
		}
	})

	t.Run("GetHealthyAgentForTopic", func(t *testing.T) {
		// Reset status to healthy
		registry.UpdateStatus(ctx, "test-agent", airegistry.AgentStatusHealthy, 0)

		agent, err := registry.GetHealthyAgentForTopic(ctx, "test.topic.something")
		if err != nil {
			t.Fatalf("Failed to get agent for topic: %v", err)
		}

		if agent.Name != "test-agent" {
			t.Errorf("Wrong agent returned: got %s, want test-agent", agent.Name)
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		err := registry.Unregister(ctx, "test-agent")
		if err != nil {
			t.Fatalf("Failed to unregister: %v", err)
		}

		_, err = registry.Get(ctx, "test-agent")
		if err != airegistry.ErrAgentNotFound {
			t.Errorf("Expected ErrAgentNotFound, got: %v", err)
		}
	})
}

// TestTaskQueueInterface tests the task queue interface (Phase 4)
// Note: Requires PostgreSQL connection
func TestTaskQueueIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	queue := aitaskqueue.NewPGStore(dbDSN, aitaskqueue.DefaultQueueConfig())
	ctx := context.Background()

	var taskID string

	t.Run("PublishTask", func(t *testing.T) {
		payload := json.RawMessage(`{"test": true}`)
		reqCtx := aitopics.RequestContext{
			ClusterID: "test-cluster",
			TenantID:  "test-tenant",
		}

		var err error
		taskID, err = queue.Publish(ctx, "test.topic", payload, reqCtx)
		if err != nil {
			t.Fatalf("Failed to publish task: %v", err)
		}

		if taskID == "" {
			t.Error("Task ID should not be empty")
		}

		t.Logf("Published task: %s", taskID)
	})

	t.Run("GetTask", func(t *testing.T) {
		if taskID == "" {
			t.Skip("No task ID from previous test")
		}

		task, err := queue.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("Failed to get task: %v", err)
		}

		if task.Status != aitaskqueue.TaskStatusPending {
			t.Errorf("Expected pending status, got: %s", task.Status)
		}

		if task.Topic != "test.topic" {
			t.Errorf("Topic mismatch: got %s, want test.topic", task.Topic)
		}
	})

	t.Run("ClaimTask", func(t *testing.T) {
		task, err := queue.ClaimTask(ctx, []string{"test.topic"}, "test-agent")
		if err != nil {
			t.Fatalf("Failed to claim task: %v", err)
		}

		if task == nil {
			t.Fatal("No task claimed")
		}

		if task.Status != aitaskqueue.TaskStatusProcessing {
			t.Errorf("Expected processing status after claim, got: %s", task.Status)
		}

		if task.AgentID != "test-agent" {
			t.Errorf("Agent ID mismatch: got %s, want test-agent", task.AgentID)
		}
	})

	t.Run("CompleteTask", func(t *testing.T) {
		if taskID == "" {
			t.Skip("No task ID from previous test")
		}

		result := &aitopics.Response{
			RequestID: taskID,
			Status:    "success",
			Code:      0,
			Message:   "Test completed",
			Payload:   json.RawMessage(`{"result": "ok"}`),
		}

		err := queue.CompleteTask(ctx, taskID, result)
		if err != nil {
			t.Fatalf("Failed to complete task: %v", err)
		}

		task, _ := queue.GetTask(ctx, taskID)
		if task.Status != aitaskqueue.TaskStatusCompleted {
			t.Errorf("Expected completed status, got: %s", task.Status)
		}
	})

	t.Run("GetResult", func(t *testing.T) {
		if taskID == "" {
			t.Skip("No task ID from previous test")
		}

		result, err := queue.GetResult(ctx, taskID)
		if err != nil {
			t.Fatalf("Failed to get result: %v", err)
		}

		if result.Status != "success" {
			t.Errorf("Result status mismatch: got %s, want success", result.Status)
		}
	})

	t.Run("ListTasks", func(t *testing.T) {
		filter := &aitaskqueue.TaskFilter{
			Topic: "test.topic",
			Limit: 10,
		}

		tasks, err := queue.ListTasks(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to list tasks: %v", err)
		}

		if len(tasks) == 0 {
			t.Error("Expected at least one task")
		}
	})

	t.Run("CountTasks", func(t *testing.T) {
		completed := aitaskqueue.TaskStatusCompleted
		filter := &aitaskqueue.TaskFilter{
			Status: &completed,
		}

		count, err := queue.CountTasks(ctx, filter)
		if err != nil {
			t.Fatalf("Failed to count tasks: %v", err)
		}

		if count == 0 {
			t.Error("Expected at least one completed task")
		}
	})

	// Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		// Delete test tasks
		count, err := queue.Cleanup(ctx, 0) // 0 means delete all completed
		if err != nil {
			t.Logf("Cleanup error (may be expected): %v", err)
		}
		t.Logf("Cleaned up %d tasks", count)
	})
}

// TestTopicRouting tests the topic router (Phase 2)
func TestTopicRouting(t *testing.T) {
	matcher := &airegistry.TopicMatcher{}

	t.Run("ExactMatch", func(t *testing.T) {
		patterns := []string{"alert.advisor.aggregate-workloads"}
		if !matcher.MatchAny(patterns, "alert.advisor.aggregate-workloads") {
			t.Error("Exact match should work")
		}
	})

	t.Run("WildcardMatch", func(t *testing.T) {
		patterns := []string{"alert.advisor.*"}
		if !matcher.MatchAny(patterns, "alert.advisor.aggregate-workloads") {
			t.Error("Wildcard match should work")
		}
		if !matcher.MatchAny(patterns, "alert.advisor.generate-suggestions") {
			t.Error("Wildcard match should work for any suffix")
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		patterns := []string{"scan.*"}
		if matcher.MatchAny(patterns, "alert.advisor.aggregate-workloads") {
			t.Error("Should not match different domain")
		}
	})

	t.Run("ExtractParts", func(t *testing.T) {
		topic := "alert.advisor.aggregate-workloads"
		
		domain := airegistry.ExtractDomain(topic)
		if domain != "alert" {
			t.Errorf("Domain mismatch: got %s, want alert", domain)
		}
		
		agent := airegistry.ExtractAgent(topic)
		if agent != "advisor" {
			t.Errorf("Agent mismatch: got %s, want advisor", agent)
		}
		
		action := airegistry.ExtractAction(topic)
		if action != "aggregate-workloads" {
			t.Errorf("Action mismatch: got %s, want aggregate-workloads", action)
		}
	})
}

// TestHealthChecker tests the health checker logic (Phase 2)
func TestHealthCheckerLogic(t *testing.T) {
	registry := airegistry.NewMemoryStore()
	ctx := context.Background()

	// Register a test agent
	agent := &airegistry.AgentRegistration{
		Name:            "health-test-agent",
		Endpoint:        "http://localhost:9999", // Non-existent
		Topics:          []string{"test.*"},
		HealthCheckPath: "/health",
		Timeout:         2 * time.Second,
		Status:          airegistry.AgentStatusUnknown,
	}
	registry.Register(ctx, agent)

	checker := airegistry.NewHealthChecker(registry, 2*time.Second, 3)

	t.Run("CheckUnhealthy", func(t *testing.T) {
		results := checker.CheckAll(ctx)
		if len(results) == 0 {
			t.Fatal("Expected at least one result")
		}

		result := results[0]
		if result.Healthy {
			t.Error("Agent with non-existent endpoint should be unhealthy")
		}

		// Verify registry was updated
		updated, _ := registry.Get(ctx, "health-test-agent")
		if updated.Status == airegistry.AgentStatusHealthy {
			t.Error("Status should not be healthy")
		}
	})
}

// Benchmark tests
func BenchmarkTaskPublish(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	queue := aitaskqueue.NewPGStore(dbDSN, aitaskqueue.DefaultQueueConfig())
	ctx := context.Background()
	payload := json.RawMessage(`{"benchmark": true}`)
	reqCtx := aitopics.RequestContext{ClusterID: "bench"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := queue.Publish(ctx, "bench.topic", payload, reqCtx)
		if err != nil {
			b.Fatalf("Publish failed: %v", err)
		}
	}
}

func BenchmarkRegistryLookup(b *testing.B) {
	registry := airegistry.NewMemoryStore()
	ctx := context.Background()

	// Register 100 agents
	for i := 0; i < 100; i++ {
		registry.Register(ctx, &airegistry.AgentRegistration{
			Name:     fmt.Sprintf("agent-%d", i),
			Endpoint: fmt.Sprintf("http://agent-%d:8080", i),
			Topics:   []string{fmt.Sprintf("topic.%d.*", i)},
			Status:   airegistry.AgentStatusHealthy,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.GetHealthyAgentForTopic(ctx, "topic.50.something")
	}
}

