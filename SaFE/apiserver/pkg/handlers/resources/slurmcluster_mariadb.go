/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// The Slinky `slurm` chart's accounting subsystem (slurmdbd) expects an external
// MariaDB and a password secret that the chart itself does not create. To make
// the "Enable accounting" toggle work out of the box, the SlurmCluster handler
// provisions a small, release-scoped MariaDB (Deployment + Service + PVC) and
// the password secret slurmdbd reads via `accounting.storageConfig`.

const (
	// mariadbImage is the database image for the accounting backend.
	mariadbImage = "docker.io/library/mariadb:11.4"
	// mariadbDatabase/mariadbUsername match the slurm chart's storageConfig defaults.
	mariadbDatabase = "slurm_acct_db"
	mariadbUsername = "slurm"
	// mariadbPasswordKey is the secret key slurmdbd reads (passwordKeyRef.key).
	mariadbPasswordKey = "password"
	// mariadbRootPasswordKey holds the MariaDB root password.
	mariadbRootPasswordKey = "root-password"
	// mariadbStorageSize is the PVC size for the accounting database.
	mariadbStorageSize = "5Gi"
	// slurmMariaDBLabel marks resources belonging to a Slurm accounting DB.
	slurmMariaDBLabel = v1.PrimusSafePrefix + "slurm.mariadb"
)

// mariadbBaseName returns the release-scoped base name for MariaDB resources.
func mariadbBaseName(release string) string { return release + "-mariadb" }

// mariadbServiceName is the Service (and slurmdbd StorageHost) for the DB.
func mariadbServiceName(release string) string { return mariadbBaseName(release) }

// mariadbSecretName is the secret holding the DB passwords (passwordKeyRef.name).
func mariadbSecretName(release string) string { return mariadbBaseName(release) + "-password" }

// ensureMariaDB provisions (idempotently) the accounting database for a release
// in the workspace namespace on the target cluster.
func (h *Handler) ensureMariaDB(ctx context.Context, clusterName, ns, release string) error {
	cs, err := h.slurmClientSet(clusterName)
	if err != nil {
		return err
	}
	name := mariadbBaseName(release)
	labels := map[string]string{
		slurmMariaDBLabel:            v1.TrueStr,
		"app.kubernetes.io/name":     "mariadb",
		"app.kubernetes.io/instance": name,
	}

	// Secret with generated passwords (create-if-absent so restarts keep the same
	// credentials the database was initialized with).
	if _, err = cs.CoreV1().Secrets(ns).Get(ctx, mariadbSecretName(release), metav1.GetOptions{}); apierrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: mariadbSecretName(release), Labels: labels},
			Type:       corev1.SecretTypeOpaque,
			StringData: map[string]string{
				mariadbPasswordKey:     randomPassword(),
				mariadbRootPasswordKey: randomPassword(),
			},
		}
		if _, err = cs.CoreV1().Secrets(ns).Create(ctx, secret, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}
	} else if err != nil {
		return err
	}

	// PVC for persistent accounting history.
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(mariadbStorageSize)},
			},
		},
	}
	if _, err = cs.CoreV1().PersistentVolumeClaims(ns).Create(ctx, pvc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// Deployment running MariaDB.
	if _, err = cs.AppsV1().Deployments(ns).Create(ctx, mariadbDeployment(name, release, labels), metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	// Service exposing MariaDB at <name>:3306 (slurmdbd StorageHost/StoragePort).
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: mariadbServiceName(release), Labels: labels},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Port:       3306,
				TargetPort: intstr.FromInt(3306),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
	if _, err = cs.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	klog.Infof("ensured MariaDB %q in namespace %q for slurm accounting", name, ns)
	return nil
}

// deleteMariaDB removes the accounting database for a release. Missing resources
// are ignored so it is safe to call on delete or when accounting is disabled.
func (h *Handler) deleteMariaDB(ctx context.Context, clusterName, ns, release string) error {
	cs, err := h.slurmClientSet(clusterName)
	if err != nil {
		return err
	}
	name := mariadbBaseName(release)
	del := metav1.DeleteOptions{}
	ignoreNotFound := func(e error) error {
		if e == nil || apierrors.IsNotFound(e) {
			return nil
		}
		return e
	}
	if err = ignoreNotFound(cs.AppsV1().Deployments(ns).Delete(ctx, name, del)); err != nil {
		return err
	}
	if err = ignoreNotFound(cs.CoreV1().Services(ns).Delete(ctx, mariadbServiceName(release), del)); err != nil {
		return err
	}
	if err = ignoreNotFound(cs.CoreV1().PersistentVolumeClaims(ns).Delete(ctx, name, del)); err != nil {
		return err
	}
	if err = ignoreNotFound(cs.CoreV1().Secrets(ns).Delete(ctx, mariadbSecretName(release), del)); err != nil {
		return err
	}
	klog.Infof("removed MariaDB %q in namespace %q", name, ns)
	return nil
}

// deleteSlurmStatesave removes the slurmctld state-save PVC(s) for a release.
// The Slinky `slurm` chart runs the controller as a StatefulSet that provisions
// these PVCs from a volumeClaimTemplate, and Kubernetes does not delete
// volumeClaimTemplate PVCs when the StatefulSet (or its helm release) is
// removed. As a result `helm uninstall` leaves the PVC behind on every delete,
// so we clean it up explicitly. A label selector is used (rather than the fixed
// `statesave-<release>-controller-0` name) so it is robust to the controller
// replica index and future naming. Missing PVCs are a no-op.
func (h *Handler) deleteSlurmStatesave(ctx context.Context, clusterName, ns, release string) error {
	cs, err := h.slurmClientSet(clusterName)
	if err != nil {
		return err
	}
	sel := fmt.Sprintf("app.kubernetes.io/instance=%s,app.kubernetes.io/name=slurmctld", release)
	err = cs.CoreV1().PersistentVolumeClaims(ns).DeleteCollection(ctx,
		metav1.DeleteOptions{}, metav1.ListOptions{LabelSelector: sel})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	klog.Infof("removed slurmctld statesave PVC(s) for release %q in namespace %q", release, ns)
	return nil
}

// scaleMariaDB sets the replica count of a release's MariaDB Deployment. It is
// used to free (0) or restore (1) the accounting database when a Slurm cluster
// is stopped or resumed. A missing Deployment is treated as a no-op.
func (h *Handler) scaleMariaDB(ctx context.Context, clusterName, ns, release string, replicas int32) error {
	cs, err := h.slurmClientSet(clusterName)
	if err != nil {
		return err
	}
	name := mariadbBaseName(release)
	scale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
	}
	_, err = cs.AppsV1().Deployments(ns).UpdateScale(ctx, name, scale, metav1.UpdateOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

// mariadbDeployment builds the MariaDB Deployment for the accounting backend.
func mariadbDeployment(name, release string, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	podLabels := map[string]string{"app": name}
	for k, v := range labels {
		podLabels[k] = v
	}
	secretName := mariadbSecretName(release)
	envFromSecret := func(key string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: secretName},
			Key:                  key,
		}}
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			// Recreate: the PVC is RWO and only one MariaDB may bind it at a time.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "mariadb",
						Image: mariadbImage,
						Ports: []corev1.ContainerPort{{ContainerPort: 3306, Name: "mysql"}},
						Env: []corev1.EnvVar{
							{Name: "MARIADB_DATABASE", Value: mariadbDatabase},
							{Name: "MARIADB_USER", Value: mariadbUsername},
							{Name: "MARIADB_PASSWORD", ValueFrom: envFromSecret(mariadbPasswordKey)},
							{Name: "MARIADB_ROOT_PASSWORD", ValueFrom: envFromSecret(mariadbRootPasswordKey)},
						},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "data",
							MountPath: "/var/lib/mysql",
						}},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								Exec: &corev1.ExecAction{Command: []string{"healthcheck.sh", "--connect", "--innodb_initialized"}},
							},
							InitialDelaySeconds: 10,
							PeriodSeconds:       10,
						},
					}},
					Volumes: []corev1.Volume{{
						Name: "data",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: name},
						},
					}},
				},
			},
		},
	}
}

// randomPassword returns a 32-hex-char (128-bit) random password.
func randomPassword() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic; fall back to a fixed-length marker
		// so provisioning still proceeds (extremely unlikely in practice).
		return "changeme-mariadb-password000000"
	}
	return hex.EncodeToString(b)
}
