/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
)

type FaultAction string

const (
	TaintAction FaultAction = "taint"

	NodeNotReady = "NotReady"
)

var k8sNodeConditionTypes = map[corev1.NodeConditionType]bool{
	corev1.NodeReady:              true,
	corev1.NodeNetworkUnavailable: true,
	corev1.NodeMemoryPressure:     true,
	corev1.NodeDiskPressure:       true,
	corev1.NodePIDPressure:        true,
}

type FaultConfig struct {
	// A unique fault ID that is consistent with the ID used by NodeAgent for monitoring.
	Id string `json:"id"`
	// Actions for handling the fault, separated by commas if there are multiple.
	Action FaultAction `json:"action,omitempty"`
	// on/off. default "off"
	Toggle string `json:"toggle,omitempty"`
	// whether the fault is auto repaired or not. default true
	IsAutoRepair *bool `json:"isAutoRepair,omitempty"`
}

func (c *FaultConfig) IsAutoRepairEnabled() bool {
	if c.IsAutoRepair == nil {
		return false
	}
	return *c.IsAutoRepair
}

func (c *FaultConfig) IsEnable() bool {
	return c.Toggle == "on"
}

// retrieves the fault configuration from a ConfigMap.
// The key is fault.id, and the value is the fault config.
func GetFaultConfigmap(ctx context.Context, cli client.Client) (map[string]*FaultConfig, error) {
	configMap := &corev1.ConfigMap{}
	err := cli.Get(ctx, client.ObjectKey{Name: common.PrimusFault, Namespace: common.PrimusSafeNamespace}, configMap)
	if err != nil {
		return make(map[string]*FaultConfig), client.IgnoreNotFound(err)
	}
	return parseFaultConfig(configMap), nil
}

func parseFaultConfig(configMap *corev1.ConfigMap) map[string]*FaultConfig {
	result := make(map[string]*FaultConfig)
	for _, val := range configMap.Data {
		conf := &FaultConfig{}
		if err := json.Unmarshal([]byte(val), conf); err != nil {
			continue
		}
		if conf.Toggle != "on" || conf.Id == "" {
			continue
		}
		if conf.IsAutoRepair == nil {
			conf.IsAutoRepair = pointer.Bool(true)
		}
		result[conf.Id] = conf
	}
	return result
}

func isShouldCreateFault(cond corev1.NodeCondition) bool {
	switch {
	case isK8sCondition(cond.Type):
		if cond.Type == corev1.NodeReady {
			if cond.Status != corev1.ConditionTrue {
				return true
			}
		} else if cond.Status == corev1.ConditionTrue {
			return true
		}
	case isPrimusCondition(cond.Type):
		return cond.Status == corev1.ConditionTrue
	}
	return false
}

func isPrimusCondition(condType corev1.NodeConditionType) bool {
	return strings.HasPrefix(string(condType), v1.PrimusSafePrefix)
}

func isK8sCondition(condType corev1.NodeConditionType) bool {
	_, ok := k8sNodeConditionTypes[condType]
	return ok
}

func listFaults(ctx context.Context, cli client.Client, labelSelector labels.Selector) ([]v1.Fault, error) {
	faultList := &v1.FaultList{}
	err := cli.List(ctx, faultList, &client.ListOptions{LabelSelector: labelSelector})
	if err != nil || len(faultList.Items) == 0 {
		return nil, err
	}
	return faultList.Items, nil
}

func createFault(ctx context.Context, cli client.Client, fault *v1.Fault) error {
	if err := cli.Create(ctx, fault); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			klog.ErrorS(err, "failed to create fault")
			return err
		} else {
			klog.Infof("the fault(%s) already exists", fault.Name)
			return nil
		}
	}
	klog.Infof("create fault, name: %s, id: %s", fault.Name, fault.Spec.MonitorId)
	return nil
}

func deleteFault(ctx context.Context, cli client.Client, fault *v1.Fault) error {
	if err := cli.Delete(ctx, fault); err != nil {
		return client.IgnoreNotFound(err)
	}
	klog.Infof("delete fault, name: %s, id: %s", fault.Name, fault.Spec.MonitorId)
	return nil
}

func generateFaultOnCreation(node *v1.FaultNode,
	cond corev1.NodeCondition, faultConfigMap map[string]*FaultConfig) *v1.Fault {
	id := getIdByConditionType(cond.Type)
	conf, ok := faultConfigMap[id]
	if !ok || conf == nil {
		return nil
	}
	return &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultName(node.AdminName, id),
			Labels: map[string]string{
				v1.ClusterIdLabel: node.ClusterName,
				v1.NodeIdLabel:    node.AdminName,
			},
		},
		Spec: v1.FaultSpec{
			MonitorId:           id,
			Message:             cond.Message,
			Action:              string(conf.Action),
			IsAutoRepairEnabled: conf.IsAutoRepairEnabled(),
			Node:                node,
		},
	}
}

func generateFaultOnDeletion(node *v1.FaultNode,
	cond corev1.NodeCondition, faultConfigMap map[string]*FaultConfig) *v1.Fault {
	if !isPrimusCondition(cond.Type) && !isK8sCondition(cond.Type) {
		return nil
	}
	id := getIdByConditionType(cond.Type)
	if conf, ok := faultConfigMap[id]; ok && !conf.IsAutoRepairEnabled() {
		return nil
	}
	return &v1.Fault{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonfaults.GenerateFaultName(node.AdminName, id),
			Labels: map[string]string{
				v1.ClusterIdLabel: node.ClusterName,
			},
		},
		Spec: v1.FaultSpec{
			MonitorId: id,
			Node:      node,
		},
	}
}

func getIdByConditionType(condType corev1.NodeConditionType) string {
	switch {
	case isPrimusCondition(condType):
		return commonfaults.GetIdByTaintKey(string(condType))
	case condType == corev1.NodeReady:
		return NodeNotReady
	default:
		return string(condType)
	}
}
