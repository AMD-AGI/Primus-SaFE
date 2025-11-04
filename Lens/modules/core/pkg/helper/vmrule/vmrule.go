package vmrule

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	VMRuleGroup    = "operator.victoriametrics.com"
	VMRuleVersion  = "v1beta1"
	VMRuleResource = "vmrules"
	VMRuleKind     = "VMRule"
	
	LabelManagedBy   = "primus-lens.amd.com/managed-by"
	LabelRuleID      = "primus-lens.amd.com/rule-id"
	LabelComponent   = "app"
	LabelCategory    = "component"
	
	ManagedByValue = "primus-lens-api"
	ComponentValue = "primus-lens"
	CategoryValue  = "alerts"
)

var vmruleGVR = schema.GroupVersionResource{
	Group:    VMRuleGroup,
	Version:  VMRuleVersion,
	Resource: VMRuleResource,
}

// VMRuleManager manages VMRule CRDs in Kubernetes
type VMRuleManager struct {
	dynamicClient dynamic.Interface
	namespace     string
}

// NewVMRuleManager creates a new VMRule manager
func NewVMRuleManager(clusterName, namespace string) (*VMRuleManager, error) {
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster clients for %s: %w", clusterName, err)
	}
	
	return &VMRuleManager{
		dynamicClient: clients.K8SClientSet.Dynamic,
		namespace:     namespace,
	}, nil
}

// CreateOrUpdateVMRule creates or updates a VMRule in Kubernetes
func (m *VMRuleManager) CreateOrUpdateVMRule(ctx context.Context, rule *model.MetricAlertRule) error {
	vmrule, err := m.buildVMRule(rule)
	if err != nil {
		return fmt.Errorf("failed to build VMRule: %w", err)
	}
	
	// Try to get existing VMRule
	existing, err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Get(ctx, rule.Name, metav1.GetOptions{})
	if err == nil {
		// Update existing VMRule
		vmrule["metadata"].(map[string]interface{})["resourceVersion"] = existing.GetResourceVersion()
		updated, err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Update(ctx, &unstructured.Unstructured{Object: vmrule}, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update VMRule: %w", err)
		}
		
		// Update UID in database model
		rule.VMRuleUID = string(updated.GetUID())
		return nil
	}
	
	// Create new VMRule
	created, err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Create(ctx, &unstructured.Unstructured{Object: vmrule}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create VMRule: %w", err)
	}
	
	// Update UID in database model
	rule.VMRuleUID = string(created.GetUID())
	return nil
}

// DeleteVMRule deletes a VMRule from Kubernetes
func (m *VMRuleManager) DeleteVMRule(ctx context.Context, name string) error {
	err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete VMRule: %w", err)
	}
	return nil
}

// GetVMRuleStatus retrieves the status of a VMRule
func (m *VMRuleManager) GetVMRuleStatus(ctx context.Context, name string) (*model.VMRuleStatus, error) {
	vmrule, err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get VMRule: %w", err)
	}
	
	// Extract status from unstructured object
	status, found, err := unstructured.NestedMap(vmrule.Object, "status")
	if err != nil || !found {
		return &model.VMRuleStatus{Phase: "Unknown"}, nil
	}
	
	// Convert to VMRuleStatus struct
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal status: %w", err)
	}
	
	var vmruleStatus model.VMRuleStatus
	if err := json.Unmarshal(statusJSON, &vmruleStatus); err != nil {
		return nil, fmt.Errorf("failed to unmarshal status: %w", err)
	}
	
	return &vmruleStatus, nil
}

// ListVMRules lists all VMRules managed by primus-lens
func (m *VMRuleManager) ListVMRules(ctx context.Context) (*unstructured.UnstructuredList, error) {
	labelSelector := fmt.Sprintf("%s=%s", LabelManagedBy, ManagedByValue)
	return m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
}

// buildVMRule builds a VMRule unstructured object from MetricAlertRule
func (m *VMRuleManager) buildVMRule(rule *model.MetricAlertRule) (map[string]interface{}, error) {
	// Parse groups from ExtType
	var groups []model.VMRuleGroup
	groupsBytes, err := json.Marshal(rule.Groups)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal groups: %w", err)
	}
	if err := json.Unmarshal(groupsBytes, &groups); err != nil {
		return nil, fmt.Errorf("failed to unmarshal groups: %w", err)
	}
	
	// Parse labels
	var labels map[string]string
	if rule.Labels != nil && len(rule.Labels) > 0 {
		labelsBytes, err := json.Marshal(rule.Labels)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal labels: %w", err)
		}
		if err := json.Unmarshal(labelsBytes, &labels); err != nil {
			return nil, fmt.Errorf("failed to unmarshal labels: %w", err)
		}
	}
	if labels == nil {
		labels = make(map[string]string)
	}
	
	// Add management labels
	labels[LabelManagedBy] = ManagedByValue
	labels[LabelRuleID] = fmt.Sprintf("%d", rule.ID)
	labels[LabelComponent] = ComponentValue
	labels[LabelCategory] = CategoryValue
	
	// Build VMRule spec
	spec := map[string]interface{}{
		"groups": groups,
	}
	
	// Build VMRule object
	vmrule := map[string]interface{}{
		"apiVersion": fmt.Sprintf("%s/%s", VMRuleGroup, VMRuleVersion),
		"kind":       VMRuleKind,
		"metadata": map[string]interface{}{
			"name":      rule.Name,
			"namespace": m.namespace,
			"labels":    labels,
			"annotations": map[string]interface{}{
				"primus-lens.amd.com/description": rule.Description,
				"primus-lens.amd.com/rule-id":     fmt.Sprintf("%d", rule.ID),
			},
		},
		"spec": spec,
	}
	
	return vmrule, nil
}

// EnableVMRule enables a VMRule by setting enabled label
func (m *VMRuleManager) EnableVMRule(ctx context.Context, name string, enabled bool) error {
	vmrule, err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get VMRule: %w", err)
	}
	
	labels := vmrule.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	
	if enabled {
		labels["enabled"] = "true"
	} else {
		labels["enabled"] = "false"
	}
	
	vmrule.SetLabels(labels)
	
	_, err = m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).Update(ctx, vmrule, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update VMRule: %w", err)
	}
	
	return nil
}

// GetVMRuleByID retrieves a VMRule by rule ID label
func (m *VMRuleManager) GetVMRuleByID(ctx context.Context, ruleID int64) (*unstructured.Unstructured, error) {
	labelSelector := fmt.Sprintf("%s=%d,%s=%s", LabelRuleID, ruleID, LabelManagedBy, ManagedByValue)
	list, err := m.dynamicClient.Resource(vmruleGVR).Namespace(m.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list VMRules: %w", err)
	}
	
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("VMRule not found for rule ID %d", ruleID)
	}
	
	return &list.Items[0], nil
}

