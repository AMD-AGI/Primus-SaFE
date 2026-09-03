/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package node

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
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

	WATCH_RETRY_INTERVAL = 3 * time.Second
	WATCH_RETRY_MAX      = 30 * time.Second
	// WATCH_HEALTHY_DURATION is the connection lifetime that clears the
	// backoff. Anything shorter counts as a failed connection, so proxies
	// dropping idle connections keep backing the agent off instead of
	// looping over Get and Watch.
	WATCH_HEALTHY_DURATION = 2 * time.Minute
	// WATCH_MIN_TIMEOUT_SECONDS is the lower bound of a single watch. The
	// real timeout is randomized within [min, 2*min) so agents do not
	// reconnect in lockstep.
	WATCH_MIN_TIMEOUT_SECONDS int64 = 300
	// WATCH_CLOSE_WARN_COUNT raises the log level after this many consecutive
	// short-lived watches.
	WATCH_CLOSE_WARN_COUNT = 3
)

// Node represents a Kubernetes node with additional functionality for monitoring and updating node status
type Node struct {
	ctx       context.Context
	k8sNode   *corev1.Node
	mu        sync.RWMutex
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
	if n == nil || n.snapshotK8sNode() == nil {
		return fmt.Errorf("please initialize node first")
	}
	klog.Infof("begin to start node watcher: %s", n.snapshotK8sNode().Name)
	if err := n.updateStartTime(); err != nil {
		klog.ErrorS(err, "failed to update start time")
	}
	go n.update()
	return nil
}

// update watches the current node and reconnects after watch failures.
func (n *Node) update() {
	consecutiveShort := 0
	for {
		if n.ctx.Err() != nil {
			n.logWatcherStop()
			return
		}
		started := time.Now()
		err := n.watchK8sNode()
		if n.ctx.Err() != nil {
			n.logWatcherStop()
			return
		}
		lived := time.Since(started)
		if err != nil {
			klog.ErrorS(err, "failed to watch k8s node")
		}
		delay := time.Duration(0)
		if lived < WATCH_HEALTHY_DURATION {
			consecutiveShort++
			delay = watchRetryDelay(consecutiveShort)
			n.logWatchClosed(lived, consecutiveShort, err)
		} else {
			consecutiveShort = 0
			if err != nil {
				delay = watchRetryDelay(1)
			}
			klog.V(4).InfoS("node watch closed, reconnecting", "duration", lived)
		}
		if n.waitRetry(delay) {
			n.logWatcherStop()
			return
		}
	}
}

// watchRetryDelay returns a jittered exponential delay for consecutive short watches.
func watchRetryDelay(consecutiveShort int) time.Duration {
	if consecutiveShort <= 0 {
		return 0
	}
	d := WATCH_RETRY_INTERVAL
	for i := 1; i < consecutiveShort; i++ {
		if d >= WATCH_RETRY_MAX/2 {
			d = WATCH_RETRY_MAX
			break
		}
		d *= 2
	}
	if d > WATCH_RETRY_MAX {
		d = WATCH_RETRY_MAX
	}
	return wait.Jitter(d, 0.5)
}

func (n *Node) waitRetry(d time.Duration) bool {
	if d <= 0 {
		return n.ctx.Err() != nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-n.ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}

func (n *Node) logWatchClosed(lived time.Duration, consecutive int, err error) {
	if consecutive >= WATCH_CLOSE_WARN_COUNT {
		klog.InfoS("node watch closed after a short connection, backing off",
			"duration", lived, "consecutive", consecutive, "err", err)
		return
	}
	klog.V(4).InfoS("node watch closed after a short connection, backing off",
		"duration", lived, "consecutive", consecutive)
}

func (n *Node) logWatcherStop() {
	node := n.snapshotK8sNode()
	if node != nil {
		klog.Infof("stop node watcher: %s", node.Name)
	}
}

// watchK8sNode watches only the current node and updates the local cache.
func (n *Node) watchK8sNode() error {
	if err := n.syncK8sNode(); err != nil {
		return err
	}

	node := n.snapshotK8sNode()
	if node == nil {
		return fmt.Errorf("please initialize node first")
	}

	timeout := watchTimeoutSeconds()
	watcher, err := n.k8sClient.Nodes().Watch(n.ctx, metav1.ListOptions{
		FieldSelector:   fields.OneTermEqualSelector("metadata.name", node.Name).String(),
		ResourceVersion: node.ResourceVersion,
		TimeoutSeconds:  &timeout,
	})
	if err != nil {
		return err
	}
	defer watcher.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return n.ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return nil
			}
			if err := n.applyWatchEvent(event); err != nil {
				return err
			}
		}
	}
}

// watchTimeoutSeconds randomizes the watch timeout so agents reconnect at
// different times instead of in lockstep.
func watchTimeoutSeconds() int64 {
	base := WATCH_MIN_TIMEOUT_SECONDS
	if base <= 0 {
		return base
	}
	return base + rand.Int64N(base)
}

// applyWatchEvent applies a watch event for the current node to the local cache.
func (n *Node) applyWatchEvent(event watch.Event) error {
	switch event.Type {
	case watch.Added, watch.Modified:
		node, ok := event.Object.(*corev1.Node)
		if !ok || node == nil {
			return fmt.Errorf("unexpected watch object: %T", event.Object)
		}
		n.setK8sNode(node)
	case watch.Deleted:
		klog.InfoS("watched node was deleted")
	case watch.Error:
		return apierrors.FromObject(event.Object)
	}
	return nil
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
	node := n.snapshotK8sNode()
	if node == nil {
		return nil
	}
	for i, currentCond := range node.Status.Conditions {
		if conditionType == string(currentCond.Type) {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}

// FindCondition finds a node condition using a custom comparison function.
func (n *Node) FindCondition(cond *corev1.NodeCondition, isCondEqual func(cond1, cond2 *corev1.NodeCondition) bool) *corev1.NodeCondition {
	node := n.snapshotK8sNode()
	if node == nil {
		return nil
	}
	for i, currentCond := range node.Status.Conditions {
		if isCondEqual(&currentCond, cond) {
			return &node.Status.Conditions[i]
		}
	}
	return nil
}

// UpdateConditions updates the node's status conditions with retry logic for conflict handling.
func (n *Node) UpdateConditions(conditions []corev1.NodeCondition) error {
	if n.snapshotK8sNode() == nil {
		return fmt.Errorf("please initialize node first")
	}
	var err error
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		k8sNode := n.snapshotK8sNode()
		if k8sNode == nil {
			return fmt.Errorf("please initialize node first")
		}
		sentResourceVersion := k8sNode.ResourceVersion
		k8sNode.Status.Conditions = conditions

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
		n.setK8sNodeIfUnchanged(sentResourceVersion, node)
		return nil
	})

	return err
}

// updateNodeStartTime updates the node's startup time label.
func (n *Node) updateNodeStartTime(startTime time.Time) error {
	k8sNode := n.snapshotK8sNode()
	if k8sNode == nil {
		return fmt.Errorf("please initialize node first")
	}
	startTimeStr := strconv.FormatInt(startTime.Unix(), 10)
	if v1.GetNodeStartupTime(k8sNode) == startTimeStr {
		return nil
	}
	data := fmt.Sprintf(`{"metadata":{"labels":{"%s": "%s"}}}`, v1.NodeStartupTimeLabel, startTimeStr)
	patched, err := n.k8sClient.Nodes().Patch(n.ctx,
		k8sNode.Name, apitypes.MergePatchType, []byte(data), metav1.PatchOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	n.setK8sNodeIfUnchanged(k8sNode.ResourceVersion, patched)
	return nil
}

// GetK8sNode returns a snapshot of the current Kubernetes node object.
func (n *Node) GetK8sNode() *corev1.Node {
	return n.snapshotK8sNode()
}

func (n *Node) snapshotK8sNode() *corev1.Node {
	if n == nil {
		return nil
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	if n.k8sNode == nil {
		return nil
	}
	return n.k8sNode.DeepCopy()
}

func (n *Node) setK8sNode(node *corev1.Node) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if node == nil {
		n.k8sNode = nil
		return
	}
	n.k8sNode = node.DeepCopy()
}

// setK8sNodeIfUnchanged stores an API response only when the cache still holds
// the resource version the request was built from. A watch event applied while
// the request was in flight is newer and must not be rolled back.
func (n *Node) setK8sNodeIfUnchanged(expectedResourceVersion string, node *corev1.Node) bool {
	if node == nil {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.k8sNode != nil && n.k8sNode.ResourceVersion != expectedResourceVersion {
		klog.V(4).InfoS("skip stale node response",
			"cached", n.k8sNode.ResourceVersion, "expected", expectedResourceVersion)
		return false
	}
	n.k8sNode = node.DeepCopy()
	return true
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
	node := n.snapshotK8sNode()
	if node == nil {
		return resource.Quantity{}
	}
	var result resource.Quantity
	switch {
	case isAmdGpu(node):
		result, _ = node.Status.Allocatable[common.AmdGpu]
	case isNvGpu(node):
		result, _ = node.Status.Allocatable[common.NvidiaGpu]
	}
	return result
}

// GetEphemeralStorage returns the allocatable EphemeralStorage for the node.
func (n *Node) GetEphemeralStorage() resource.Quantity {
	node := n.snapshotK8sNode()
	if node == nil {
		return resource.Quantity{}
	}
	result, _ := node.Status.Allocatable[corev1.ResourceEphemeralStorage]
	return result
}

// isNvGpu checks if the node has NVIDIA GPU hardware.
func (n *Node) isNvGpu() bool {
	return isNvGpu(n.snapshotK8sNode())
}

// isAmdGpu checks if the node has AMD GPU hardware.
func (n *Node) isAmdGpu() bool {
	return isAmdGpu(n.snapshotK8sNode())
}

func isNvGpu(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	_, ok := node.Labels[common.NvidiaIdentification]
	return ok
}

func isAmdGpu(node *corev1.Node) bool {
	if node == nil {
		return false
	}
	val, ok := node.Labels[common.AMDGpuIdentification]
	return ok && val == v1.TrueStr
}

// syncK8sNode synchronizes the local node cache with the latest version from Kubernetes API.
func (n *Node) syncK8sNode() error {
	current := n.snapshotK8sNode()
	if current == nil {
		return fmt.Errorf("please initialize node first")
	}
	k8sNode, err := n.k8sClient.Nodes().Get(n.ctx, current.Name, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get k8s node")
		return err
	}
	n.setK8sNode(k8sNode)
	return nil
}

// getLocation retrieves the system timezone using "timedatectl" command.
func getLocation() (*time.Location, error) {
	cmd := fmt.Sprintf(`%s timedatectl |grep "Time zone" |awk -F" " '{print $3}'`, NSENTER)
	statusCode, output := utils.ExecuteCommand(cmd, 30*time.Second)
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
	statusCode, output := utils.ExecuteCommand(cmd, 30*time.Second)
	if statusCode != types.StatusOk {
		return time.Time{}, fmt.Errorf("failed to do 'uptime -s', output: %s", output)
	}
	startTime, err := time.ParseInLocation(time.DateTime, output, loc)
	if err != nil {
		return time.Time{}, err
	}
	return startTime.UTC(), nil
}
