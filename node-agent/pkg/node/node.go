/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package node

import (
	"context"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
)

const (
	nsenter = "nsenter --target 1 --mount --uts --ipc --net --pid --"
)

var (
	sleepTime = 5 * time.Second
)

type Node struct {
	k8sNode   *corev1.Node
	k8sClient typedcorev1.CoreV1Interface
	tomb      *channel.Tomb
	isExited  bool
}

func NewNode(opts *types.Options) (*Node, error) {
	k8sClientSet, _, err := commonclient.NewClientSetInCluster()
	if err != nil {
		klog.ErrorS(err, "failed to new ClientSet in cluster")
		return nil, err
	}
	return NewNodeWithClientSet(opts, k8sClientSet)
}

func NewNodeWithClientSet(opts *types.Options, k8sClientSet kubernetes.Interface) (*Node, error) {
	n := &Node{
		tomb:     channel.NewTomb(),
		isExited: true,
	}
	n.k8sClient = k8sClientSet.CoreV1()
	var err error
	n.k8sNode, err = n.k8sClient.Nodes().Get(context.Background(), opts.NodeName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "failed to get node")
		return nil, err
	}
	return n, nil
}

func (n *Node) Start() error {
	if n == nil || n.k8sNode == nil {
		return fmt.Errorf("please initialize node first")
	}
	klog.Infof("begin to start node watcher: %s", n.k8sNode.Name)
	if err := n.updateStartTime(); err != nil {
		klog.ErrorS(err, "failed to update start time")
	}
	go n.update()
	n.isExited = false
	return nil
}

func (n *Node) Stop() {
	if !n.IsExited() && n.tomb != nil {
		n.tomb.Stop()
		if n.k8sNode != nil {
			klog.Infof("the node watcher is stopped: %s", n.k8sNode.Name)
		}
	}
	n.isExited = true
}

func (n *Node) update() {
	defer func() {
		n.tomb.Done()
	}()

	for {
		select {
		case <-n.tomb.Stopping():
			return
		default:
			k8sNode, err := n.k8sClient.Nodes().Get(context.Background(), n.GetK8sNode().Name, metav1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "failed to get node")
			} else {
				n.k8sNode = k8sNode
			}
			time.Sleep(sleepTime)
		}
	}
}

// Use the shell command(uptime -s) to obtain the startup time of the node
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

func (n *Node) IsMatchChip(chip string) bool {
	switch chip {
	case types.AmdGpuChip:
		return n.isAmdGpu()
	case types.NvidiaGpuChip:
		return n.isNvGpu()
	case "":
		return true
	default:
		return false
	}
}

func (n *Node) IsExited() bool {
	return n.isExited
}

func (n *Node) FindCondition(conditionType string) *corev1.NodeCondition {
	if n.k8sNode == nil {
		return nil
	}
	for i, cond := range n.k8sNode.Status.Conditions {
		if string(cond.Type) == conditionType {
			return &n.k8sNode.Status.Conditions[i]
		}
	}
	return nil
}

func (n *Node) UpdateConditions(conditions []corev1.NodeCondition) error {
	n.k8sNode.Status.Conditions = conditions
	node, err := n.k8sClient.Nodes().UpdateStatus(context.Background(), n.k8sNode, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	n.k8sNode = node
	return nil
}

func (n *Node) updateNodeStartTime(startTime time.Time) error {
	startTimeStr := strconv.FormatInt(startTime.Unix(), 10)
	if v1.GetNodeStartupTime(n.k8sNode) == startTimeStr {
		return nil
	}
	data := fmt.Sprintf(`{"metadata":{"labels":{"%s": "%s"}}}`, v1.NodeStartupTimeLabel, startTimeStr)
	k8sNode, err := n.k8sClient.Nodes().Patch(context.Background(),
		n.k8sNode.Name, apitypes.MergePatchType, []byte(data), metav1.PatchOptions{})
	if err != nil {
		return client.IgnoreNotFound(err)
	}
	n.k8sNode = k8sNode
	return nil
}

func (n *Node) GetK8sNode() *corev1.Node {
	return n.k8sNode
}

func (n *Node) GetGpuQuantity() resource.Quantity {
	if n.k8sNode == nil {
		return resource.Quantity{}
	}
	var gpuQuantity resource.Quantity
	switch {
	case n.isAmdGpu():
		gpuQuantity, _ = n.k8sNode.Status.Allocatable[common.AmdGpu]
	case n.isNvGpu():
		gpuQuantity, _ = n.k8sNode.Status.Allocatable[common.NvidiaGpu]
	}
	return gpuQuantity
}

func (n *Node) isNvGpu() bool {
	if n.k8sNode == nil {
		return false
	}
	_, ok1 := n.k8sNode.Labels[common.NvidiaIdentification]
	_, ok2 := n.k8sNode.Labels[common.NvidiaVfio]
	return ok1 && !ok2
}

func (n *Node) isAmdGpu() bool {
	if n.k8sNode == nil {
		return false
	}
	val, ok := n.k8sNode.Labels[common.AMDGpuIdentification]
	return ok && val == "true"
}

func getLocation() (*time.Location, error) {
	cmd := fmt.Sprintf(`%s timedatectl |grep "Time zone" |awk -F" " '{print $3}'`, nsenter)
	statusCode, resp := utils.ExecuteCommand(cmd, 0)
	if statusCode != types.StatusOk {
		return nil, fmt.Errorf("fail to execute command, resp: %s", resp)
	}
	timezone := resp
	if timezone == "" {
		timezone = "UTC"
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		klog.ErrorS(err, "failed to load location. Use utc instead of it", "timezone", timezone)
		timezone = "UTC"
		loc, _ = time.LoadLocation(timezone)
	}
	klog.Infof("current node location: %s", timezone)
	return loc, nil
}

func getUptime(loc *time.Location) (time.Time, error) {
	cmd := fmt.Sprintf("%s uptime -s", nsenter)
	statusCode, resp := utils.ExecuteCommand(cmd, 0)
	if statusCode != types.StatusOk {
		return time.Time{}, fmt.Errorf("fail to do 'uptime -s', resp: %s", resp)
	}
	startTime, err := time.ParseInLocation(time.DateTime, resp, loc)
	if err != nil {
		return time.Time{}, err
	}
	return startTime.UTC(), nil
}
