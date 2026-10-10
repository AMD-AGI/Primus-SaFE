/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"strings"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// ExternalWorkspaceAffinityTerms builds OR nodeSelectorTerms for current and legacy
// workspace label keys. Autopilot accounts for lease remaining from
// activeDeadlineSeconds; SaFE does not write absolute lease-end affinity.
func ExternalWorkspaceAffinityTerms(workspace string) []interface{} {
	prefixes := []string{v1.ExternalWorkspaceLabel, v1.ExternalWorkspaceLabelLegacy}
	terms := make([]interface{}, 0, len(prefixes))
	for _, key := range prefixes {
		terms = append(terms, map[string]interface{}{
			"matchExpressions": []interface{}{
				map[string]interface{}{
					"key":      key,
					"operator": "In",
					"values":   []interface{}{workspace},
				},
			},
		})
	}
	return terms
}

// ExternalWorkspaceAffinityKeys are matchExpression keys owned by ExternalWorkspaceAffinityTerms.
func ExternalWorkspaceAffinityKeys() map[string]struct{} {
	return map[string]struct{}{
		v1.ExternalWorkspaceLabel:       {},
		v1.ExternalWorkspaceLabelLegacy: {},
		// Drop legacy lease-end expressions if a template still carries them.
		v1.ExternalLeaseEndLabel:       {},
		v1.ExternalLeaseEndLabelLegacy: {},
	}
}

// ExternalCustomerMatchExpressions builds node affinity expressions from CustomerLabels
// for the kube-scheduler path (specified_nodes / excluded_nodes / custom labels).
func ExternalCustomerMatchExpressions(workload *v1.Workload) []interface{} {
	if workload == nil || len(workload.Spec.CustomerLabels) == 0 {
		return nil
	}
	var result []interface{}
	for key, val := range workload.Spec.CustomerLabels {
		var values []interface{}
		for _, part := range strings.Fields(val) {
			values = append(values, part)
		}
		if len(values) == 0 {
			continue
		}
		operator := "In"
		switch key {
		case common.ExcludedNodes:
			key = v1.K8sHostName
			operator = "NotIn"
		case v1.K8sHostName, common.SpecifiedNodes:
			affinity, ok := workload.Annotations[v1.NodesAffinityAnnotation]
			if ok && affinity != common.NodesAffinityRequired {
				continue
			}
			key = v1.K8sHostName
		}
		result = append(result, map[string]interface{}{
			"key":      key,
			"operator": operator,
			"values":   values,
		})
	}
	return result
}

// MergeExternalWorkspaceAffinity ANDs each workspace term into every existing term.
func MergeExternalWorkspaceAffinity(existing, workspaceTerms []interface{}) []interface{} {
	managed := ExternalWorkspaceAffinityKeys()
	if len(existing) == 0 {
		return workspaceTerms
	}
	out := make([]interface{}, 0, len(existing)*len(workspaceTerms))
	for _, raw := range existing {
		base, ok := raw.(map[string]interface{})
		if !ok {
			base = map[string]interface{}{}
		}
		baseExprs := stripManagedMatchExpressions(base["matchExpressions"], managed)
		baseFields, hasFields := base["matchFields"]
		for _, wsRaw := range workspaceTerms {
			ws, ok := wsRaw.(map[string]interface{})
			if !ok {
				continue
			}
			wsExprs, _ := ws["matchExpressions"].([]interface{})
			merged := map[string]interface{}{
				"matchExpressions": append(append([]interface{}{}, baseExprs...), wsExprs...),
			}
			if hasFields {
				merged["matchFields"] = baseFields
			}
			out = append(out, merged)
		}
	}
	return out
}

func stripManagedMatchExpressions(raw interface{}, managed map[string]struct{}) []interface{} {
	exprs, ok := raw.([]interface{})
	if !ok || len(exprs) == 0 {
		return nil
	}
	out := make([]interface{}, 0, len(exprs))
	for _, entry := range exprs {
		expr, ok := entry.(map[string]interface{})
		if !ok {
			out = append(out, entry)
			continue
		}
		key, _ := expr["key"].(string)
		if _, drop := managed[key]; drop {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// ExternalActiveDeadlineSeconds returns the pod activeDeadlineSeconds for batch
// workloads that declare a timeout, matching dispatcher modifyActiveDeadline.
func ExternalActiveDeadlineSeconds(workload *v1.Workload) (int64, bool) {
	if workload == nil || !runsToCompletionKind(workload.SpecKind()) {
		return 0, false
	}
	timeout := int64(workload.GetTimeout())
	if timeout <= 0 {
		return 0, false
	}
	return timeout, true
}

func runsToCompletionKind(kind string) bool {
	switch kind {
	case common.AuthoringKind, common.PytorchJobKind, common.UnifiedJobKind, common.TorchFTKind,
		common.JobKind, common.MonarchClient, common.SandboxKind:
		return true
	}
	return false
}

// ExternalGangNodeSelectorTerms is the nodeAffinity used by ProvisioningRequest
// PodTemplates: customer constraints ANDed with workspace terms.
func ExternalGangNodeSelectorTerms(workload *v1.Workload) []interface{} {
	ws := ""
	if workload != nil {
		ws = workload.Spec.Workspace
	}
	workspaceTerms := ExternalWorkspaceAffinityTerms(ws)
	customer := ExternalCustomerMatchExpressions(workload)
	if len(customer) == 0 {
		return workspaceTerms
	}
	existing := []interface{}{
		map[string]interface{}{"matchExpressions": customer},
	}
	return MergeExternalWorkspaceAffinity(existing, workspaceTerms)
}
