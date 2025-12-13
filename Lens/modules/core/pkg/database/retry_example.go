package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// Example 1: Directly using WithRetry to wrap a single operation
func ExampleUpdateNodeWithRetry(ctx context.Context, node *model.Node) error {
	facade := GetFacade().GetNode()

	// Original code (without retry):
	// return facade.UpdateNode(ctx, node)

	// Add automatic retry (recommended):
	return WithRetry(ctx, func() error {
		return facade.UpdateNode(ctx, node)
	})
}

// Example 2: Using custom retry configuration
func ExampleUpdateNodeWithCustomRetry(ctx context.Context, node *model.Node) error {
	facade := GetFacade().GetNode()

	customConfig := RetryConfig{
		MaxRetries:    5,                // Maximum 5 retries
		InitialDelay:  1 * time.Second,  // Initial delay 1 second
		MaxDelay:      10 * time.Second, // Maximum delay 10 seconds
		DelayMultiple: 2.0,              // Exponential backoff multiplier
	}

	return WithRetryConfig(ctx, customConfig, func() error {
		return facade.UpdateNode(ctx, node)
	})
}

// Example 3: Using retry in batch operations
func ExampleBatchUpdateNodesWithRetry(ctx context.Context, nodes []*model.Node) error {
	facade := GetFacade().GetNode()

	for _, node := range nodes {
		// Each node update has independent retry mechanism
		err := WithRetry(ctx, func() error {
			return facade.UpdateNode(ctx, node)
		})
		if err != nil {
			return err // Or continue processing other nodes depending on business requirements
		}
	}

	return nil
}

// Example 4: Using retry in complex business logic
func ExampleComplexOperationWithRetry(ctx context.Context, nodeName string, gpuCount int32) error {
	// Wrap entire business logic in retry function
	return WithRetry(ctx, func() error {
		facade := GetFacade().GetNode()

		// 1. Query node (read operations usually don't need retry as they're not affected by master-slave failover)
		node, err := facade.GetNodeByName(ctx, nodeName)
		if err != nil {
			return err
		}
		if node == nil {
			return ErrNodeNotFound
		}

		// 2. Modify node information
		node.GpuCount = gpuCount

		// 3. Update to database (write operation, will automatically retry on failure)
		return facade.UpdateNode(ctx, node)
	})
}

// Example 5: Using RetryableOperation to create reusable retry wrappers
func ExampleRetryableOperationPattern() {
	facade := GetFacade().GetNode()

	// Create a retryable UpdateNode function
	retryableUpdateNode := RetryableOperation(facade.UpdateNode)

	// Use it anywhere, will automatically retry
	// retryableUpdateNode(ctx, node)

	// Can also create other retryable operations
	retryableCreateNode := RetryableOperation(facade.CreateNode)
	// retryableCreateNode(ctx, newNode)

	_ = retryableUpdateNode
	_ = retryableCreateNode
}

// Example 6: Using retry in async operations
func ExampleAsyncOperationWithRetry(ctx context.Context, nodes []*model.Node) []error {
	var resultChannels []<-chan error

	// Start multiple async operations
	for _, node := range nodes {
		node := node // Capture loop variable
		resultCh := WithRetryAsync(ctx, func() error {
			return GetFacade().GetNode().UpdateNode(ctx, node)
		})
		resultChannels = append(resultChannels, resultCh)
	}

	// Collect results
	var errors []error
	for _, resultCh := range resultChannels {
		if err := <-resultCh; err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// Example 7: Using in Reconciler (recommended pattern)
func ExampleReconcilerWithRetry(ctx context.Context, nodeName string) error {
	// For Reconciler or Controller scenarios, retry is recommended
	// Although Kubernetes has its own retry mechanism, adding immediate retry can reduce error logs

	facade := GetFacade().GetNode()

	// Get node
	node, err := facade.GetNodeByName(ctx, nodeName)
	if err != nil {
		return err
	}

	if node == nil {
		// Create new node with retry
		newNode := &model.Node{
			Name:   nodeName,
			Status: "Ready",
		}

		return WithRetry(ctx, func() error {
			return facade.CreateNode(ctx, newNode)
		})
	}

	// Update existing node with retry
	node.Status = "Ready"
	return WithRetry(ctx, func() error {
		return facade.UpdateNode(ctx, node)
	})
}

// Example 8: Without retry (some scenarios don't need it)
func ExampleWithoutRetry(ctx context.Context) error {
	facade := GetFacade().GetNode()

	// Scenario 1: Pure read operations are not affected by master-slave failover
	nodes, err := facade.ListGpuNodes(ctx)
	if err != nil {
		return err
	}
	_ = nodes

	// Scenario 2: User-triggered operations where immediate error feedback is desired
	// Rather than retrying multiple times in the background
	node, err := facade.GetNodeByName(ctx, "node-1")
	if err != nil {
		return err // Return error directly, let user decide whether to retry
	}
	_ = node

	return nil
}

// Custom error types
var (
	ErrNodeNotFound = NewDatabaseError("node not found")
)

type DatabaseError struct {
	message string
}

func NewDatabaseError(msg string) *DatabaseError {
	return &DatabaseError{message: msg}
}

func (e *DatabaseError) Error() string {
	return e.message
}

// Usage Recommendations Summary:
//
// 1. **Write Operations (Recommended to use retry)**:
//    - CreateNode, UpdateNode, DeleteNode
//    - CreateGpuDevice, UpdateGpuDevice, DeleteGpuDevice
//    - All Create/Update/Delete operations
//
// 2. **Read Operations (Usually no retry needed)**:
//    - GetNodeByName, ListGpuNodes, SearchNode
//    - Read operations are not affected by read-only replicas
//
// 3. **Reconciler/Controller (Strongly Recommended)**:
//    - Kubernetes will re-trigger reconcile, but adding immediate retry reduces errors
//    - Reduces unnecessary error logs
//
// 4. **User-Triggered Operations (Depends on business requirements)**:
//    - Can optionally use when API endpoints handle user requests
//    - Consider user experience to avoid excessive response times
//
// 5. **Batch Operations (Use Cautiously)**:
//    - For batch operations with many items, consider setting timeouts
//    - Or set independent retry for each item rather than retrying the entire batch
