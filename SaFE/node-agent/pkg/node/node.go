/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
)

var (
	NSENTER = "nsenter --target 1 --mount --uts --ipc --net --pid --"

	SYNC_INTERVAL = 3 * time.Second
)

// Node represents a Kubernetes node with additional functionality for monitoring and updating node status
type Node struct {
	ctx       context.Context
	k8sNode   *corev1.Node
	mu        sync.Mutex
	k8sClient typedcorev1.CoreV1Interface
}

// NewNode creates a new Node instance using in-cluster Kubernetes client configuration.
func NewNode(ctx context.Context, opts *types.Options) (*Node, error) {
	k8sClientSet, _, err := commonclient.NewClientSetInCluster()
	if err != nil {
		klog.ErrorS(err, "failed to new ClientSet in cluster")
		return nil, err
	}
	return NewNodeWithClientSet(ctx, opts, k8sClientSet)
}

// NewNodeWithClientSet creates a new Node instance with a provided Kubernetes clientset.
func NewNodeWithClientSet(ctx context.Context, opts *types.Options, k8sClientSet kubernetes.Interface) (*Node, error) {
	n := &Node{
		ctx: ctx,
	}
	n.k8sClient = k8sClientSet.CoreV1()
	var err error
	n.k8sNode, err = n.k8sClient.Nodes().Get(ctx, opts.NodeName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get node")
		return nil, err
	}
	return n, nil
}

// Start initializes and starts the node watcher goroutine.
func (n *Node) Start() error {
	n.mu.Lock()
	if n == nil || n.k8sNode == nil {
		n.mu.Unlock()
		return fmt.Errorf("please initialize node first")
	}
	nodeName := n.k8sNode.Name
	n.mu.Unlock()

	klog.Infof("begin to start node watcher: %s", nodeName)
	if err := n.updateStartTime(); err != nil {
		klog.ErrorS(err, "failed to update start time")
	}
	go n.update()
	return nil
}

// update runs continuously to sync node status at regular intervals.
func (n *Node) update() {
	for {
		select {
		case <-n.ctx.Done():
			n.mu.Lock()
			nodeName := ""
			if n.k8sNode != nil {
				nodeName = n.k8sNode.Name
			}
			n.mu.Unlock()
			if nodeName != "" {
				klog.Infof("stop node watcher: %s", nodeName)
			}
			return
		default:
			n.syncK8sNode()
			time.Sleep(SYNC_INTERVAL)
		}
	}
}

// updateStartTime updates the node's startup time by executing system commands(uptime -s).
func (n *Node) updateStartTime() error {
	loc, err := getLocation()
	if err != nil {
		klog.ErrorS(err, "failed to get location")
		return err
	}
	uptime, err := getUptime(loc)
	if err != nil || uptime.IsZero() {
		klog.ErrorS(err, "failed to get uptime")
		return err
	}
	if err = n.updateNodeStartTime(uptime); err != nil {
		klog.ErrorS(err, "failed to update node startTime")
		return err
	}
	klog.Infof("node start time: %s", uptime.Format(time.RFC3339))
	return nil
}

// FindConditionByType finds a node condition by its type string.
func (n *Node) FindConditionByType(conditionType string) *corev1.NodeCondition {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.k8sNode == nil {
		return nil
	}
	for i, currentCond := range n.k8sNode.Status.Conditions {
		if conditionType == string(currentCond.Type) {
			cond := n.k8sNode.Status.Conditions[i]
			return &cond
		}
	}
	return nil
}

// FindCondition finds a node condition using a custom comparison function.
func (n *Node) FindCondition(cond *corev1.NodeCondition, isCondEqual func(cond1, cond2 *corev1.NodeCondition) bool) *corev1.NodeCondition {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.k8sNode == nil {
		return nil
	}
	for i, currentCond := range n.k8sNode.Status.Conditions {
		if isCondEqual(&currentCond, cond) {
			foundCond := n.k8sNode.Status.Conditions[i]
			return &foundCond
		}
	}
	return nil
}

// UpdateConditions updates the node's status conditions with retry logic for conflict handling.
func (n *Node) UpdateConditions(conditions []corev1.NodeCondition) error {
	n.mu.Lock()
	if n.k8sNode == nil {
		n.mu.Unlock()
		return fmt.Errorf("please initialize node first")
	}
	n.mu.Unlock()

	var err error
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		n.mu.Lock()
		k8sNode := n.k8sNode.DeepCopy()
		k8sNode.Status.Conditions = conditions
		n.mu.Unlock()

		node, updateErr := n.k8sClient.Nodes().UpdateStatus(n.ctx, k8sNode, metav1.UpdateOptions{})
		if updateErr != nil {
			if apierrors.IsConflict(updateErr) {
				// refresh node
				if err = n.syncK8sNode(); err != nil {
					return err
				}
			}
			return updateErr
		}
		n.mu.Lock()
		n.k8sNode = node.DeepCopy()
		n.mu.Unlock()
		return nil
	})

	return err
}

// updateNodeStartTime updates the node's startup time label.
func (n *Node) updateNodeStartTime(startTime time.Time) error {
	n.mu.Lock()
	nodeName := n.k8sNode.Name
	currentStartTime := v1.GetNodeStartupTime(n.k8sNode)
	n.mu.Unlock()

	startTimeStr := strconv.FormatInt(startTime.Unix(), 10)
	if currentStartTime == startTimeStr {
		return nil
	}
	data := fmt.Sprintf(`{"metadata":{"labels":{"%s": "%s"}}}`, v1.NodeStartupTimeLabel, startTimeStr)
	k8sNode, err := n.k8sClient.Nodes().Patch(n.ctx,
		nodeName, apitypes.MergePatchType, []byte(data), metav1.PatchOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	n.mu.Lock()
	n.k8sNode = k8sNode
	n.mu.Unlock()
	return nil
}

// GetK8sNode returns a deep copy of the current Kubernetes node object.
// It uses mutex lock to prevent data races during concurrent access.
func (n *Node) GetK8sNode() *corev1.Node {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.k8sNode.DeepCopy()
}

// IsMatchGpuChip checks if the node's GPU chip matches the specified chip type.
func (n *Node) IsMatchGpuChip(chip string) bool {
	switch chip {
	case string(v1.AmdGpuChip):
		return n.isAmdGpu()
	case string(v1.NvidiaGpuChip):
		return n.isNvGpu()
	case "":
		return true
	default:
		return false
	}
}

// GetGpuQuantity returns the allocatable GPU quantity for the node.
func (n *Node) GetGpuQuantity() resource.Quantity {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.k8sNode == nil {
		return resource.Quantity{}
	}
	var gpuQuantity resource.Quantity
	switch {
	case n.isAmdGpuLocked():
		gpuQuantity, _ = n.k8sNode.Status.Allocatable[common.AmdGpu]
	case n.isNvGpuLocked():
		gpuQuantity, _ = n.k8sNode.Status.Allocatable[common.NvidiaGpu]
	}
	return gpuQuantity
}

// isNvGpu checks if the node has NVIDIA GPU hardware.
func (n *Node) isNvGpu() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.isNvGpuLocked()
}

// isNvGpuLocked checks if the node has NVIDIA GPU hardware (must be called with lock held).
func (n *Node) isNvGpuLocked() bool {
	if n.k8sNode == nil {
		return false
	}
	_, ok := n.k8sNode.Labels[common.NvidiaIdentification]
	return ok
}

// isAmdGpu checks if the node has AMD GPU hardware.
func (n *Node) isAmdGpu() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.isAmdGpuLocked()
}

// isAmdGpuLocked checks if the node has AMD GPU hardware (must be called with lock held).
func (n *Node) isAmdGpuLocked() bool {
	if n.k8sNode == nil {
		return false
	}
	val, ok := n.k8sNode.Labels[common.AMDGpuIdentification]
	return ok && val == v1.TrueStr
}

// syncK8sNode synchronizes the local node cache with the latest version from Kubernetes API.
func (n *Node) syncK8sNode() error {
	n.mu.Lock()
	nodeName := n.k8sNode.Name
	n.mu.Unlock()

	k8sNode, err := n.k8sClient.Nodes().Get(n.ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get k8s node")
		return err
	}

	n.mu.Lock()
	n.k8sNode = k8sNode.DeepCopy()
	n.mu.Unlock()
	return nil
}

// getLocation retrieves the system timezone using "timedatectl" command.
func getLocation() (*time.Location, error) {
	cmd := fmt.Sprintf(`%s timedatectl |grep "Time zone" |awk -F" " '{print $3}'`, NSENTER)
	statusCode, output := utils.ExecuteCommand(cmd, 0)
	if statusCode != types.StatusOk {
		return nil, fmt.Errorf("failed to execute command, output: %s", output)
	}
	timezone := output
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		klog.ErrorS(err, "failed to load location. Use utc instead of it")
		timezone = "UTC"
		loc, _ = time.LoadLocation(timezone)
	}
	klog.Infof("current node location: %s", timezone)
	return loc, nil
}

// getUptime gets the system uptime using the "uptime -s" command.
func getUptime(loc *time.Location) (time.Time, error) {
	cmd := fmt.Sprintf("%s uptime -s", NSENTER)
	statusCode, output := utils.ExecuteCommand(cmd, 0)
	if statusCode != types.StatusOk {
		return time.Time{}, fmt.Errorf("failed to do 'uptime -s', output: %s", output)
	}
	startTime, err := time.ParseInLocation(time.DateTime, output, loc)
	if err != nil {
		return time.Time{}, err
	}
	return startTime.UTC(), nil
}
