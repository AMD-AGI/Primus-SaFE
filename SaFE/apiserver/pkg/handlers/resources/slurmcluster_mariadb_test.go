/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestMariadbNameHelpers(t *testing.T) {
	assert.Equal(t, "slurm-c1-mariadb", mariadbBaseName("slurm-c1"))
	assert.Equal(t, "slurm-c1-mariadb", mariadbServiceName("slurm-c1"))
	assert.Equal(t, "slurm-c1-mariadb-password", mariadbSecretName("slurm-c1"))
}

func TestRandomPassword(t *testing.T) {
	a := randomPassword()
	b := randomPassword()
	assert.Len(t, a, 32)
	assert.Len(t, b, 32)
	assert.NotEqual(t, a, b)
}

func TestMariadbDeployment(t *testing.T) {
	labels := map[string]string{slurmMariaDBLabel: v1.TrueStr}
	dep := mariadbDeployment("slurm-c1-mariadb", "slurm-c1", labels)
	assert.Equal(t, "slurm-c1-mariadb", dep.Name)
	assert.EqualValues(t, 1, *dep.Spec.Replicas)
	assert.Equal(t, "slurm-c1-mariadb", dep.Spec.Selector.MatchLabels["app"])

	container := dep.Spec.Template.Spec.Containers[0]
	assert.Equal(t, mariadbImage, container.Image)
	// Password env vars pull from the release-scoped secret.
	var passwordRef *corev1.EnvVarSource
	for _, e := range container.Env {
		if e.Name == "MARIADB_PASSWORD" {
			passwordRef = e.ValueFrom
		}
	}
	assert.NotNil(t, passwordRef)
	assert.Equal(t, mariadbSecretName("slurm-c1"), passwordRef.SecretKeyRef.Name)
	// Recreate strategy so the RWO PVC is never double-bound.
	assert.Equal(t, "Recreate", string(dep.Spec.Strategy.Type))
}

func TestEnsureAndDeleteMariaDB(t *testing.T) {
	ctx := context.Background()
	ns := testSlurmWorkspace
	release := slurmReleaseName("mycluster")
	cs := k8sfake.NewSimpleClientset()
	h, _ := slurmHandlerWithDataplane(cs, testSlurmCluster)

	assert.NoError(t, h.ensureMariaDB(ctx, testSlurmCluster, ns, release))

	// Secret, PVC, Deployment and Service are created.
	_, err := cs.CoreV1().Secrets(ns).Get(ctx, mariadbSecretName(release), metav1.GetOptions{})
	assert.NoError(t, err)
	_, err = cs.CoreV1().PersistentVolumeClaims(ns).Get(ctx, mariadbBaseName(release), metav1.GetOptions{})
	assert.NoError(t, err)
	_, err = cs.AppsV1().Deployments(ns).Get(ctx, mariadbBaseName(release), metav1.GetOptions{})
	assert.NoError(t, err)
	_, err = cs.CoreV1().Services(ns).Get(ctx, mariadbServiceName(release), metav1.GetOptions{})
	assert.NoError(t, err)

	// Idempotent: a second ensure does not error.
	assert.NoError(t, h.ensureMariaDB(ctx, testSlurmCluster, ns, release))

	// Delete removes everything.
	assert.NoError(t, h.deleteMariaDB(ctx, testSlurmCluster, ns, release))
	_, err = cs.AppsV1().Deployments(ns).Get(ctx, mariadbBaseName(release), metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))
	_, err = cs.CoreV1().Secrets(ns).Get(ctx, mariadbSecretName(release), metav1.GetOptions{})
	assert.True(t, apierrors.IsNotFound(err))

	// Delete is a no-op when nothing exists.
	assert.NoError(t, h.deleteMariaDB(ctx, testSlurmCluster, ns, release))
}

func TestScaleMariaDB(t *testing.T) {
	ctx := context.Background()
	ns := testSlurmWorkspace
	release := slurmReleaseName("mycluster")
	cs := k8sfake.NewSimpleClientset()
	h, _ := slurmHandlerWithDataplane(cs, testSlurmCluster)

	// Missing deployment -> no-op, no error.
	assert.NoError(t, h.scaleMariaDB(ctx, testSlurmCluster, ns, release, 0))

	// After provisioning, scaling succeeds.
	assert.NoError(t, h.ensureMariaDB(ctx, testSlurmCluster, ns, release))
	assert.NoError(t, h.scaleMariaDB(ctx, testSlurmCluster, ns, release, 0))
}

func TestDeleteSlurmStatesave(t *testing.T) {
	ctx := context.Background()
	ns := testSlurmWorkspace
	release := slurmReleaseName("mycluster")
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name:      "statesave-" + release + "-controller-0",
		Namespace: ns,
		Labels: map[string]string{
			"app.kubernetes.io/instance": release,
			"app.kubernetes.io/name":     "slurmctld",
		},
	}}
	cs := k8sfake.NewSimpleClientset(pvc)
	h, _ := slurmHandlerWithDataplane(cs, testSlurmCluster)

	// DeleteCollection with the slurmctld label selector succeeds. (The fake
	// clientset does not implement DeleteCollection filtering, so this asserts
	// the call path rather than actual object removal.)
	assert.NoError(t, h.deleteSlurmStatesave(ctx, testSlurmCluster, ns, release))

	// No matching PVC -> no error.
	assert.NoError(t, h.deleteSlurmStatesave(ctx, testSlurmCluster, ns, "slurm-absent"))
}

// TestGetSlurmPodUsage exercises the full accounting path: a running slurmd
// worker pod on a workspace node contributes its requests to the per-node and
// aggregate usage.
func TestGetSlurmPodUsage(t *testing.T) {
	ctx := context.Background()
	ws := genSlurmWorkspace(testSlurmWorkspace, testSlurmCluster, nil)

	adminNode := genAvailableNode("adm1", testSlurmCluster, testSlurmWorkspace, testSlurmFlavor, genMockNodeResource(64, 128<<30, 8))
	adminNode.Status.MachineStatus.HostName = "knode1"

	worker := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      slurmReleaseName("mycluster") + "-pool1-0",
			Namespace: testSlurmWorkspace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of":   "slurm",
				"app.kubernetes.io/component": "worker",
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "knode1",
			Containers: []corev1.Container{{
				Name: "slurmd",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("8Gi"),
					},
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	cs := k8sfake.NewSimpleClientset(worker)
	h, _ := slurmHandlerWithDataplane(cs, testSlurmCluster, ws, adminNode)

	perNode, total, nodes, err := h.getSlurmPodUsage(ctx, ws)
	assert.NoError(t, err)
	assert.Len(t, nodes, 1)
	assert.Equal(t, "adm1", nodes[0])
	assert.Equal(t, "4", total.Cpu().String())
	assert.Contains(t, perNode, "adm1")
}
