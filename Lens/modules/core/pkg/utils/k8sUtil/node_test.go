package k8sUtil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeReady(t *testing.T) {
	t.Run("node ready", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		result := NodeReady(node)
		assert.True(t, result)
	})

	t.Run("node not ready", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		result := NodeReady(node)
		assert.False(t, result)
	})

	t.Run("node status unknown", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionUnknown,
					},
				},
			},
		}

		result := NodeReady(node)
		assert.False(t, result)
	})

	t.Run("no ready condition", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeMemoryPressure,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		result := NodeReady(node)
		assert.False(t, result)
	})

	t.Run("empty conditions list", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{},
			},
		}

		result := NodeReady(node)
		assert.False(t, result)
	})

	t.Run("multiple conditions but only Ready is True", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeMemoryPressure,
						Status: corev1.ConditionFalse,
					},
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   corev1.NodeDiskPressure,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		result := NodeReady(node)
		assert.True(t, result)
	})
}

func TestNodeStatus(t *testing.T) {
	t.Run("node status is Ready", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		status := NodeStatus(node)
		assert.Equal(t, NodeStatusReady, status)
	})

	t.Run("node status is NotReady", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		status := NodeStatus(node)
		assert.Equal(t, NodeStatusNotReady, status)
	})

	t.Run("node status is Unknown", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionUnknown,
					},
				},
			},
		}

		status := NodeStatus(node)
		assert.Equal(t, NodeStatusUnknown, status)
	})

	t.Run("no ready condition returns Unknown", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeMemoryPressure,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		status := NodeStatus(node)
		assert.Equal(t, NodeStatusUnknown, status)
	})

	t.Run("empty conditions list returns Unknown", func(t *testing.T) {
		node := corev1.Node{
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{},
			},
		}

		status := NodeStatus(node)
		assert.Equal(t, NodeStatusUnknown, status)
	})

	t.Run("verify constant values", func(t *testing.T) {
		assert.Equal(t, "Ready", NodeStatusReady)
		assert.Equal(t, "NotReady", NodeStatusNotReady)
		assert.Equal(t, "Unknown", NodeStatusUnknown)
	})
}

func TestNodeStatusComprehensive(t *testing.T) {
	t.Run("complete node object", func(t *testing.T) {
		node := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:               corev1.NodeMemoryPressure,
						Status:             corev1.ConditionFalse,
						LastHeartbeatTime:  metav1.Now(),
						LastTransitionTime: metav1.Now(),
						Reason:             "KubeletHasSufficientMemory",
						Message:            "kubelet has sufficient memory available",
					},
					{
						Type:               corev1.NodeDiskPressure,
						Status:             corev1.ConditionFalse,
						LastHeartbeatTime:  metav1.Now(),
						LastTransitionTime: metav1.Now(),
						Reason:             "KubeletHasNoDiskPressure",
						Message:            "kubelet has no disk pressure",
					},
					{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionTrue,
						LastHeartbeatTime:  metav1.Now(),
						LastTransitionTime: metav1.Now(),
						Reason:             "KubeletReady",
						Message:            "kubelet is posting ready status",
					},
				},
			},
		}

		assert.True(t, NodeReady(node))
		assert.Equal(t, NodeStatusReady, NodeStatus(node))
	})

	t.Run("problematic node", func(t *testing.T) {
		node := corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "problematic-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeMemoryPressure,
						Status: corev1.ConditionTrue,
						Reason: "KubeletHasInsufficientMemory",
					},
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
						Reason: "KubeletNotReady",
					},
				},
			},
		}

		assert.False(t, NodeReady(node))
		assert.Equal(t, NodeStatusNotReady, NodeStatus(node))
	})
}

