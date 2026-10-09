/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dispatcher

import (
	"strconv"
	"testing"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

var affinityTestPath = []string{"spec", "template", "spec", "affinity", "nodeAffinity",
	"requiredDuringSchedulingIgnoredDuringExecution", "nodeSelectorTerms"}

func expr(key, op string, values ...string) interface{} {
	vals := make([]interface{}, 0, len(values))
	for _, v := range values {
		vals = append(vals, v)
	}
	return map[string]interface{}{"key": key, "operator": op, "values": vals}
}

func term(exprs ...interface{}) interface{} {
	return map[string]interface{}{"matchExpressions": append([]interface{}{}, exprs...)}
}

// injectedTerms stands in for terms another layer already wrote into the pod template:
// each confines the pod to a pool of nodes, and the terms are ORed with each other.
func injectedTerms(n int) []interface{} {
	terms := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		terms = append(terms, term(
			expr("example.com/pool", "In", "pool-"+strconv.Itoa(i)),
			expr("example.com/lease-end", "Gt", "100"),
		))
	}
	return terms
}

func affinityObject(terms []interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{}}
	if terms != nil {
		_ = jobutils.SetNestedField(obj.Object, terms, affinityTestPath)
	}
	return obj
}

func affinityWorkload(workspace string, labels map[string]string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w", Annotations: map[string]string{}}}
	w.Spec.Workspace = workspace
	w.Spec.CustomerLabels = labels
	return w
}

func renderedTerms(t *testing.T, obj *unstructured.Unstructured) []interface{} {
	terms, _, err := jobutils.NestedSlice(obj.Object, affinityTestPath)
	assert.NilError(t, err)
	return terms
}

// exprMatches evaluates one node selector requirement against node labels the way the
// scheduler does for the operators used here.
func exprMatches(t *testing.T, raw interface{}, labels map[string]string) bool {
	e := raw.(map[string]interface{})
	key := e["key"].(string)
	var values []string
	if vs, ok := e["values"].([]interface{}); ok {
		for _, v := range vs {
			values = append(values, v.(string))
		}
	}
	val, has := labels[key]
	switch e["operator"] {
	case "In":
		for _, v := range values {
			if has && v == val {
				return true
			}
		}
		return false
	case "NotIn":
		for _, v := range values {
			if has && v == val {
				return false
			}
		}
		return true
	case "Gt":
		got, err1 := strconv.ParseInt(val, 10, 64)
		want, err2 := strconv.ParseInt(values[0], 10, 64)
		return has && err1 == nil && err2 == nil && got > want
	}
	t.Fatalf("unexpected operator %v", e["operator"])
	return false
}

// schedulable reports whether a node with these labels satisfies the required node
// affinity: terms are ORed, the expressions inside a term are ANDed.
func schedulable(t *testing.T, terms []interface{}, labels map[string]string) bool {
	if len(terms) == 0 {
		return true
	}
	for _, raw := range terms {
		exprs, _ := raw.(map[string]interface{})["matchExpressions"].([]interface{})
		ok := true
		for _, e := range exprs {
			if !exprMatches(t, e, labels) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func node(name, workspace, pool, leaseEnd string, extra map[string]string) map[string]string {
	labels := map[string]string{v1.K8sHostName: name, "example.com/pool": pool, "example.com/lease-end": leaseEnd}
	if workspace != "" {
		labels[v1.WorkspaceIdLabel] = workspace
	}
	for k, v := range extra {
		labels[k] = v
	}
	return labels
}

func TestRequiredAffinityWithoutCustomerLabelsIsUnchanged(t *testing.T) {
	// No template terms: one term with the workspace confinement.
	obj := affinityObject(nil)
	assert.NilError(t, modifyRequiredNodeAffinity(obj, affinityWorkload("ws-1", nil), affinityTestPath))
	assert.DeepEqual(t, renderedTerms(t, obj), []interface{}{
		term(expr(v1.WorkspaceIdLabel, "In", "ws-1")),
	})

	// Several template terms: only the first gains the workspace confinement, as before.
	obj = affinityObject(injectedTerms(3))
	assert.NilError(t, modifyRequiredNodeAffinity(obj, affinityWorkload("ws-1", nil), affinityTestPath))
	want := injectedTerms(3)
	first := want[0].(map[string]interface{})
	first["matchExpressions"] = append(first["matchExpressions"].([]interface{}),
		expr(v1.WorkspaceIdLabel, "In", "ws-1"))
	assert.DeepEqual(t, renderedTerms(t, obj), want)

	// Default workspace and no labels: nothing to add, the template is left alone.
	obj = affinityObject(injectedTerms(2))
	assert.NilError(t, modifyRequiredNodeAffinity(obj, affinityWorkload("default", nil), affinityTestPath))
	assert.DeepEqual(t, renderedTerms(t, obj), injectedTerms(2))
}

func TestRequiredAffinityOnlyCustomerLabels(t *testing.T) {
	obj := affinityObject(nil)
	w := affinityWorkload("default", map[string]string{"example.com/gpu-model": "model-a"})
	assert.NilError(t, modifyRequiredNodeAffinity(obj, w, affinityTestPath))
	assert.DeepEqual(t, renderedTerms(t, obj), []interface{}{
		term(expr("example.com/gpu-model", "In", "model-a")),
	})
}

func TestRequiredAffinityCustomerLabelsWithOneInjectedTerm(t *testing.T) {
	obj := affinityObject(injectedTerms(1))
	w := affinityWorkload("ws-1", map[string]string{"example.com/gpu-model": "model-a"})
	assert.NilError(t, modifyRequiredNodeAffinity(obj, w, affinityTestPath))
	want := injectedTerms(1)
	first := want[0].(map[string]interface{})
	first["matchExpressions"] = append(first["matchExpressions"].([]interface{}),
		expr(v1.WorkspaceIdLabel, "In", "ws-1"), expr("example.com/gpu-model", "In", "model-a"))
	assert.DeepEqual(t, renderedTerms(t, obj), want)
}

func TestRequiredAffinityCustomerLabelsWithSeveralInjectedTerms(t *testing.T) {
	obj := affinityObject(injectedTerms(3))
	w := affinityWorkload("ws-1", map[string]string{"example.com/gpu-model": "model-a"})
	assert.NilError(t, modifyRequiredNodeAffinity(obj, w, affinityTestPath))
	terms := renderedTerms(t, obj)
	assert.Equal(t, len(terms), 3)
	want := injectedTerms(3)
	for i := range want {
		m := want[i].(map[string]interface{})
		exprs := m["matchExpressions"].([]interface{})
		if i == 0 {
			exprs = append(exprs, expr(v1.WorkspaceIdLabel, "In", "ws-1"))
		}
		m["matchExpressions"] = append(exprs, expr("example.com/gpu-model", "In", "model-a"))
	}
	assert.DeepEqual(t, terms, want)

	// A node outside the label is rejected whichever term it would match.
	other := map[string]string{"example.com/gpu-model": "model-b"}
	same := map[string]string{"example.com/gpu-model": "model-a"}
	for pool := 1; pool < 3; pool++ {
		p := "pool-" + strconv.Itoa(pool)
		assert.Assert(t, !schedulable(t, terms, node("node-x", "", p, "200", other)), p)
		assert.Assert(t, schedulable(t, terms, node("node-x", "", p, "200", same)), p)
	}
}

func TestRequiredAffinityHostnamePinHoldsAcrossInjectedTerms(t *testing.T) {
	obj := affinityObject(injectedTerms(3))
	w := affinityWorkload("ws-1", map[string]string{v1.K8sHostName: "node-b"})
	assert.NilError(t, modifyRequiredNodeAffinity(obj, w, affinityTestPath))
	terms := renderedTerms(t, obj)

	// node-a is in an injected pool and has lease left, but the user pinned node-b.
	for pool := 0; pool < 3; pool++ {
		p := "pool-" + strconv.Itoa(pool)
		assert.Assert(t, !schedulable(t, terms, node("node-a", "ws-1", p, "200", nil)), p)
	}
	// node-b still schedules through any injected pool (term 0 also needs the workspace).
	assert.Assert(t, schedulable(t, terms, node("node-b", "ws-1", "pool-0", "200", nil)))
	assert.Assert(t, schedulable(t, terms, node("node-b", "", "pool-2", "200", nil)))
	// The injected constraints still hold for the pinned node.
	assert.Assert(t, !schedulable(t, terms, node("node-b", "ws-1", "pool-9", "200", nil)))
	assert.Assert(t, !schedulable(t, terms, node("node-b", "ws-1", "pool-1", "50", nil)))

	// specified/excluded nodes keys are rewritten to hostname In / NotIn in every term.
	obj = affinityObject(injectedTerms(2))
	w = affinityWorkload("default", map[string]string{common.ExcludedNodes: "node-a node-c"})
	assert.NilError(t, modifyRequiredNodeAffinity(obj, w, affinityTestPath))
	terms = renderedTerms(t, obj)
	assert.Assert(t, !schedulable(t, terms, node("node-a", "", "pool-1", "200", nil)))
	assert.Assert(t, schedulable(t, terms, node("node-b", "", "pool-1", "200", nil)))

	// A preferred (non-required) hostname choice adds no required expression anywhere.
	obj = affinityObject(injectedTerms(2))
	w = affinityWorkload("default", map[string]string{v1.K8sHostName: "node-b"})
	w.Annotations[v1.NodesAffinityAnnotation] = common.NodesAffinityPreferred
	assert.NilError(t, modifyRequiredNodeAffinity(obj, w, affinityTestPath))
	assert.DeepEqual(t, renderedTerms(t, obj), injectedTerms(2))
}
