/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	gcrv1 "github.com/google/go-containerregistry/pkg/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/ops_job/exportimage"
	rmutils "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
)

const (
	// Default concurrent workers for image export
	exportImageDefaultConcurrent = 3

	// The registry's CA, for a registry signed by a private CA. Optional.
	harborTLSNamespace  = "harbor"
	harborTLSSecretName = "harbor-tls"
)

// ExportImageJobReconciler saves a workload's running container as an image. It reads the
// container through pods/exec and pushes from this process, so it works the same on every
// kind of node and the registry credential never enters the user's container.
type ExportImageJobReconciler struct {
	*OpsJobBaseReconciler
	dbClient dbclient.Interface
	*controller.Controller[string]
	// export is exportimage.Export; tests replace it.
	export func(ctx context.Context, req exportimage.Request) (*exportimage.Result, error)
}

// SetupExportImageJobController initializes and registers ExportImageJobReconciler with the controller manager
func SetupExportImageJobController(ctx context.Context, mgr manager.Manager) error {
	// Check if database is enabled
	if !commonconfig.IsDBEnable() {
		klog.Infof("Database is not enabled, skip ExportImageJobController setup")
		return nil
	}

	// Create reconciler instance
	r := &ExportImageJobReconciler{
		OpsJobBaseReconciler: &OpsJobBaseReconciler{
			Client:        mgr.GetClient(),
			clientManager: utils.NewObjectManagerSingleton(),
		},
		dbClient: dbclient.NewClient(),
		export:   exportimage.Export,
	}

	// Verify database client initialization
	if r.dbClient == nil {
		return fmt.Errorf("failed to initialize database client for ExportImageJobController")
	}

	// Initialize worker controller for parallel processing
	r.Controller = controller.NewController[string](r, exportImageDefaultConcurrent)
	r.start(ctx)

	// Register controller to watch OpsJob resources
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.OpsJob{}, builder.WithPredicates(predicate.Or(
			predicate.GenerationChangedPredicate{},
			onFirstPhaseChangedPredicate(),
		))).
		Complete(r)

	if err != nil {
		return fmt.Errorf("failed to setup ExportImageJobController: %w", err)
	}

	klog.Infof("Setup ExportImageJobController successfully with %d workers", exportImageDefaultConcurrent)
	return nil
}

// start initializes and runs the worker routines for processing export jobs
func (r *ExportImageJobReconciler) start(ctx context.Context) {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
}

// Reconcile is the main reconciliation loop for ExportImageJob resources
func (r *ExportImageJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	return r.OpsJobBaseReconciler.Reconcile(ctx, req, r)
}

// observe checks if the job is still processing or completed
func (r *ExportImageJobReconciler) observe(ctx context.Context, job *v1.OpsJob) (bool, error) {
	// For direct execution model, observe doesn't need to check external Job status
	return false, nil
}

// filter determines whether to process the OpsJob, returns true to skip
func (r *ExportImageJobReconciler) filter(_ context.Context, job *v1.OpsJob) bool {
	return job.Spec.Type != v1.OpsJobExportImageType
}

// handle processes pending export jobs by adding them to worker queue
func (r *ExportImageJobReconciler) handle(ctx context.Context, job *v1.OpsJob) (ctrlruntime.Result, error) {
	if job.IsPending() {
		// Update job phase to running
		if err := r.setJobPhase(ctx, job, v1.OpsJobRunning); err != nil {
			return ctrlruntime.Result{}, fmt.Errorf("failed to set job phase to running: %w", err)
		}

		// Add to worker queue for async processing
		r.Add(job.Name)

		// Ensure job will be reconciled on timeout
		return newRequeueAfterResult(job), nil
	}
	return ctrlruntime.Result{}, nil
}

// Do exports the workload's main container.
func (r *ExportImageJobReconciler) Do(ctx context.Context, jobName string) (ctrlruntime.Result, error) {
	job := &v1.OpsJob{}
	if err := r.Get(ctx, client.ObjectKey{Name: jobName}, job); err != nil {
		klog.ErrorS(err, "failed to get OpsJob", "jobName", jobName)
		return ctrlruntime.Result{}, err
	}
	if job.IsEnd() {
		return ctrlruntime.Result{}, nil
	}
	if left := job.GetLeftTime(); left > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(left)*time.Second)
		defer cancel()
	}
	outputs, err := r.exportJob(ctx, job)
	if err != nil {
		klog.ErrorS(err, "failed to export image", "job", job.Name)
		return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobFailed, err.Error(), nil)
	}
	return ctrlruntime.Result{}, r.setJobCompleted(ctx, job, v1.OpsJobSucceeded, "Image exported successfully", outputs)
}

// exportJob runs one export and returns the job's outputs. Every error it returns is the
// job's failure message.
func (r *ExportImageJobReconciler) exportJob(ctx context.Context, job *v1.OpsJob) ([]v1.Parameter, error) {
	workloadId := getWorkloadIdFromJob(job)
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workload ID is empty")
	}
	sourceImage := getSourceImageFromJob(job)
	if sourceImage == "" {
		return nil, commonerrors.NewBadRequest("source image is empty")
	}
	workload := &v1.Workload{}
	if err := r.Get(ctx, client.ObjectKey{Name: workloadId}, workload); err != nil {
		return nil, fmt.Errorf("failed to get workload: %w", err)
	}
	podName := pickExportPod(commonworkload.PodsOf(ctx, r.dbClient, workload))
	if podName == "" {
		return nil, commonerrors.NewBadRequest("workload has no pods")
	}

	defaultRegistry, err := r.dbClient.GetDefaultRegistryInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get default registry: %w", err)
	}
	if defaultRegistry == nil || defaultRegistry.URL == "" {
		return nil, commonerrors.NewBadRequest("default push registry not exist, please contact your administrator")
	}
	targetImage, err := generateTargetImageName(sourceImage, time.Now(), randomSuffix())
	if err != nil {
		return nil, err
	}
	fullTargetImage := fmt.Sprintf("%s/%s", defaultRegistry.URL, targetImage)
	target, err := name.NewTag(fullTargetImage)
	if err != nil {
		return nil, fmt.Errorf("invalid target image %s: %w", fullTargetImage, err)
	}

	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, v1.GetClusterId(workload))
	if err != nil {
		return nil, err
	}
	namespace := v1.GetWorkspaceId(workload)
	pod, err := k8sClients.ClientSet().CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get pod %s/%s: %w", namespace, podName, err)
	}
	containerName, status, err := exportContainer(pod, commonworkload.GetMainContainerByPod(workload, workload.SpecKind(), podName))
	if err != nil {
		return nil, err
	}
	keychain, transport, err := r.registryAccess(ctx)
	if err != nil {
		return nil, err
	}
	// The base image may come from a registry only the workload's own pull secrets open.
	// They are consulted after the platform's credential, so they never decide how the
	// target registry is written to.
	keychain = authn.NewMultiKeychain(keychain, podPullKeychain(ctx, k8sClients.ClientSet(), pod))

	klog.Infof("Starting image export: workload=%s, pod=%s/%s, container=%s, target=%s",
		workloadId, namespace, podName, containerName, fullTargetImage)
	res, err := r.export(ctx, exportimage.Request{
		Exec: &exportimage.PodExecer{
			Config:    k8sClients.RestConfig(),
			Client:    k8sClients.ClientSet(),
			Namespace: namespace,
			Pod:       podName,
			Container: containerName,
		},
		ImageID:   status.ImageID,
		StartedAt: status.State.Running.StartedAt.Time,
		Target:    target,
		Keychain:  keychain,
		Transport: transport,
		Platform:  gcrv1.Platform{OS: "linux", Architecture: "amd64"},
		Logf: func(format string, args ...any) {
			klog.Infof("export %s: "+format, append([]any{job.Name}, args...)...)
		},
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("Exported image: workload=%s, target=%s, digest=%s, base=%s, changed=%d, deleted=%d",
		workloadId, fullTargetImage, res.Digest, res.Base, res.Changed, res.Deleted)
	return []v1.Parameter{
		{Name: "status", Value: "Completed"},
		{Name: "target", Value: fullTargetImage},
		{Name: "digest", Value: res.Digest},
		{Name: "message", Value: "Image exported successfully"},
	}, nil
}

// pickExportPod returns the first running pod, or the first pod at all.
func pickExportPod(pods []v1.WorkloadPod) string {
	for _, p := range pods {
		if p.PodId != "" && p.Phase == corev1.PodRunning {
			return p.PodId
		}
	}
	if len(pods) > 0 {
		return pods[0].PodId
	}
	return ""
}

// exportContainer returns the container to export (the main container, else the first)
// and its status, which must be running.
func exportContainer(pod *corev1.Pod, mainContainer string) (string, *corev1.ContainerStatus, error) {
	containerName := mainContainer
	if containerName == "" {
		if len(pod.Spec.Containers) == 0 {
			return "", nil, fmt.Errorf("pod %s has no containers", pod.Name)
		}
		containerName = pod.Spec.Containers[0].Name
	}
	for i := range pod.Status.ContainerStatuses {
		st := &pod.Status.ContainerStatuses[i]
		if st.Name != containerName {
			continue
		}
		if st.State.Running == nil {
			return "", nil, commonerrors.NewBadRequest(fmt.Sprintf("container %s of pod %s is not running", containerName, pod.Name))
		}
		return containerName, st, nil
	}
	return "", nil, fmt.Errorf("pod %s has no status for container %s", pod.Name, containerName)
}

// registryAccess returns the credentials and trust the push uses: the platform's image
// import credential and, if present, the registry's private CA. Both stay in this process.
func (r *ExportImageJobReconciler) registryAccess(ctx context.Context) (authn.Keychain, http.RoundTripper, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      common.ImageImportSecretName,
		Namespace: common.PrimusSafeNamespace,
	}, secret); err != nil {
		return nil, nil, fmt.Errorf("failed to get secret %s: %w", common.ImageImportSecretName, err)
	}
	configData, ok := secret.Data["config.json"]
	if !ok {
		return nil, nil, fmt.Errorf("config.json not found in secret %s", common.ImageImportSecretName)
	}
	keychain, err := exportimage.NewConfigKeychain(configData)
	if err != nil {
		return nil, nil, err
	}

	var ca []byte
	caSecret := &corev1.Secret{}
	err = r.Get(ctx, apitypes.NamespacedName{Namespace: harborTLSNamespace, Name: harborTLSSecretName}, caSecret)
	switch {
	case err == nil:
		ca = caSecret.Data["ca.crt"]
	case !apierrors.IsNotFound(err):
		return nil, nil, fmt.Errorf("failed to get secret %s/%s: %w", harborTLSNamespace, harborTLSSecretName, err)
	}
	transport, err := exportimage.NewTransport(ca)
	if err != nil {
		return nil, nil, err
	}
	return keychain, transport, nil
}

// podPullKeychain reads the pod's image pull secrets. One that cannot be read or parsed
// is skipped: it only matters if the base image is not readable otherwise, and that
// failure is reported when the base is read.
func podPullKeychain(ctx context.Context, cs kubernetes.Interface, pod *corev1.Pod) authn.Keychain {
	var chains []authn.Keychain
	for _, ref := range pod.Spec.ImagePullSecrets {
		secret, err := cs.CoreV1().Secrets(pod.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			klog.Warningf("export: cannot read image pull secret %s/%s: %v", pod.Namespace, ref.Name, err)
			continue
		}
		data, ok := secret.Data[corev1.DockerConfigJsonKey]
		if !ok {
			continue
		}
		kc, err := exportimage.NewConfigKeychain(data)
		if err != nil {
			klog.Warningf("export: cannot parse image pull secret %s/%s: %v", pod.Namespace, ref.Name, err)
			continue
		}
		chains = append(chains, kc)
	}
	return authn.NewMultiKeychain(chains...)
}

// generateTargetImageName returns the target image without the registry host:
// custom/<namespace>/<repository>:<YYYYMMDDHHMMSS>-<suffix>. Two exports of the same
// image in the same second still get different tags, so no export overwrites another.
// Example: "harbor.example.com/proxy/rocm/7.0-preview:tag" -> "custom/rocm/7.0-preview:20250112093000-1a2b3c"
func generateTargetImageName(sourceImage string, now time.Time, suffix string) (string, error) {
	// Harbor requires lowercase repository names, and only the repository is kept.
	ref, err := name.ParseReference(strings.ToLower(sourceImage))
	if err != nil {
		return "", fmt.Errorf("invalid source image format: %s", sourceImage)
	}
	parts := strings.Split(ref.Context().RepositoryStr(), "/")
	repository := parts[len(parts)-1]
	namespace := "library"
	if len(parts) >= 2 {
		namespace = parts[len(parts)-2]
	}
	return fmt.Sprintf("%s/%s/%s:%s-%s",
		common.ExportImageProject,
		namespace,
		repository,
		now.UTC().Format("20060102150405"),
		suffix), nil
}

func randomSuffix() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%06x", time.Now().UnixNano()&0xffffff)
	}
	return hex.EncodeToString(b)
}

// getWorkloadIdFromJob extracts workload ID from OpsJob parameters
func getWorkloadIdFromJob(job *v1.OpsJob) string {
	param := job.GetParameter(v1.ParameterWorkload)
	if param != nil {
		return param.Value
	}
	return ""
}

// getSourceImageFromJob extracts source image from OpsJob parameters
func getSourceImageFromJob(job *v1.OpsJob) string {
	for _, param := range job.Spec.Inputs {
		if param.Name == v1.ParameterImage {
			return param.Value
		}
	}
	return ""
}
