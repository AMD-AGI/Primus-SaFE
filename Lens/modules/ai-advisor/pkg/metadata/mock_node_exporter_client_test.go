package metadata

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// MockNodeExporterClient is a mock implementation of node-exporter client
type MockNodeExporterClient struct {
	ProcessTree         *types.PodProcessTree
	GetProcessTreeErr   error
	GetProcessTreeCalls int
}

// NewMockNodeExporterClient creates a new mock node exporter client
func NewMockNodeExporterClient() *MockNodeExporterClient {
	return &MockNodeExporterClient{}
}

// GetPodProcessTree returns mocked process tree
func (m *MockNodeExporterClient) GetPodProcessTree(ctx context.Context, req *types.ProcessTreeRequest) (*types.PodProcessTree, error) {
	m.GetProcessTreeCalls++

	if m.GetProcessTreeErr != nil {
		return nil, m.GetProcessTreeErr
	}

	if m.ProcessTree != nil {
		return m.ProcessTree, nil
	}

	// Return default mock process tree
	return &types.PodProcessTree{
		PodName:        req.PodName,
		PodNamespace:   req.PodNamespace,
		PodUID:         req.PodUID,
		TotalProcesses: 5,
		TotalPython:    2,
		Containers: []*types.ContainerProcessTree{
			{
				ContainerName: "main",
				RootProcess: &types.ProcessInfo{
					HostPID:  1000,
					IsPython: false,
					Cmdline:  "/bin/bash",
					Children: []*types.ProcessInfo{
						{
							HostPID:  1001,
							IsPython: true,
							Cmdline:  "python train.py --primus --version 1.0",
							Children: nil,
						},
					},
				},
			},
		},
	}, nil
}

// SetProcessTree sets the process tree to return
func (m *MockNodeExporterClient) SetProcessTree(tree *types.PodProcessTree) {
	m.ProcessTree = tree
}

// SetError sets the error to return
func (m *MockNodeExporterClient) SetError(err error) {
	m.GetProcessTreeErr = err
}

// Reset resets the mock state
func (m *MockNodeExporterClient) Reset() {
	m.ProcessTree = nil
	m.GetProcessTreeErr = nil
	m.GetProcessTreeCalls = 0
}
