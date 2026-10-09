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

	// defaultStagingProject is the registry project the containers' layers are staged in,
	// one repository per export. It must exist, private, before images are saved.
	defaultStagingProject = "save-staging"
)

// ExportImageJobReconciler saves a workload's running container as an image. The
// container computes and uploads its own layer, with a token that can write one staging
// repository; this process only starts it through pods/exec and puts the image together
// in the registry, so no image data passes through it or the API server, and it works the
// same on every kind of node.
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

	clusterID := v1.GetClusterId(workload)
	dest, err := r.destination(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	targetPath, err := generateTargetImageName(dest.targetProject, sourceImage, time.Now(), randomSuffix())
	if err != nil {
		return nil, err
	}
	fullTargetImage := fmt.Sprintf("%s/%s", dest.registry, targetPath)
	target, err := name.NewTag(fullTargetImage)
	if err != nil {
		return nil, fmt.Errorf("invalid target image %s: %w", fullTargetImage, err)
	}
	// One repository per export, for the container's layer.
	staging, err := name.NewRepository(fmt.Sprintf("%s/%s/%s", dest.registry, dest.stagingProject, job.Name))
	if err != nil {
		return nil, fmt.Errorf("invalid staging repository: %w", err)
	}

	k8sClients, err := rmutils.GetK8sClientFactory(r.clientManager, clusterID)
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
	access, err := r.registryAccess(ctx, dest.caSecret, dest.stagingPushSecret)
	if err != nil {
		return nil, err
	}
	klog.Infof("Starting image export: workload=%s, pod=%s/%s, container=%s, staging=%s, target=%s",
		workloadId, namespace, podName, containerName, staging, fullTargetImage)
	res, err := r.export(ctx, exportimage.Request{
		Exec: &exportimage.PodExecer{
			Config:    k8sClients.RestConfig(),
			Client:    k8sClients.ClientSet(),
			Namespace: namespace,
			Pod:       podName,
			Container: containerName,
		},
		ImageID:         status.ImageID,
		Staging:         staging,
		Target:          target,
		Keychain:        access.keychain,
		StagingKeychain: access.stagingKeychain,
		Transport:       access.transport,
		CA:              access.ca,
		Platform:        gcrv1.Platform{OS: "linux", Architecture: "amd64"},
		Logf: func(format string, args ...any) {
			klog.Infof("export %s: "+format, append([]any{job.Name}, args...)...)
		},
	})
	if err != nil {
		return nil, err
	}
	klog.Infof("Exported image: workload=%s, target=%s, digest=%s, base=%s, changed=%d, deleted=%d",
		workloadId, fullTargetImage, res.Digest, res.Base, res.Layer.Changed, res.Layer.Deleted)
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

// exportDestination is where one cluster's saved images go.
type exportDestination struct {
	registry          string
	targetProject     string
	stagingProject    string
	caSecret          string
	stagingPushSecret string
}

// destination reads where a cluster's saved images go. A cluster without settings saves
// to the default registry.
func (r *ExportImageJobReconciler) destination(ctx context.Context, clusterID string) (*exportDestination, error) {
	cfg, _, err := commonconfig.GetSaveImageCluster(clusterID)
	if err != nil {
		return nil, err
	}
	d := &exportDestination{
		registry:          cfg.Registry,
		targetProject:     cfg.TargetProject,
		stagingProject:    cfg.StagingProject,
		caSecret:          cfg.CASecret,
		stagingPushSecret: cfg.StagingPushSecret,
	}
	if d.registry == "" {
		defaultRegistry, err := r.dbClient.GetDefaultRegistryInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get default registry: %w", err)
		}
		if defaultRegistry == nil || defaultRegistry.URL == "" {
			return nil, commonerrors.NewBadRequest("default push registry not exist, please contact your administrator")
		}
		d.registry = defaultRegistry.URL
	}
	if d.targetProject == "" {
		d.targetProject = common.ExportImageProject
	}
	if d.stagingProject == "" {
		d.stagingProject = defaultStagingProject
	}
	if d.stagingPushSecret == "" {
		d.stagingPushSecret = common.PrimusSafeNamespace + "/" + common.SaveImageStagingSecretName
	}
	return d, nil
}

// registryCredentials is this process's access to the registries.
type registryCredentials struct {
	keychain authn.Keychain
	// stagingKeychain mints the container's upload token; see exportimage.Request.
	stagingKeychain authn.Keychain
	transport       http.RoundTripper
	// ca is the CA the container checks the registry against.
	ca []byte
}

// registryAccess returns the credentials and trust the export uses: the platform's image
// import credential, the built-in registry's private CA if there is one, the configured
// registry CA ("<namespace>/<name>", key ca.crt) if there is one, and the staging-only
// credential. The platform's credential stays in this process; only the CA and a token
// minted with the staging-only credential go to the container.
func (r *ExportImageJobReconciler) registryAccess(ctx context.Context, caSecret, stagingPushSecret string) (*registryCredentials, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, apitypes.NamespacedName{
		Name:      common.ImageImportSecretName,
		Namespace: common.PrimusSafeNamespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", common.ImageImportSecretName, err)
	}
	configData, ok := secret.Data["config.json"]
	if !ok {
		return nil, fmt.Errorf("config.json not found in secret %s", common.ImageImportSecretName)
	}
	keychain, err := exportimage.NewConfigKeychain(configData)
	if err != nil {
		return nil, err
	}

	var builtinCA []byte
	tlsSecret := &corev1.Secret{}
	err = r.Get(ctx, apitypes.NamespacedName{Namespace: harborTLSNamespace, Name: harborTLSSecretName}, tlsSecret)
	switch {
	case err == nil:
		builtinCA = tlsSecret.Data["ca.crt"]
	case !apierrors.IsNotFound(err):
		return nil, fmt.Errorf("failed to get secret %s/%s: %w", harborTLSNamespace, harborTLSSecretName, err)
	}
	ca := builtinCA
	if caSecret != "" {
		ns, n, ok := strings.Cut(caSecret, "/")
		if !ok || ns == "" || n == "" {
			return nil, fmt.Errorf("the registry CA secret %q is not <namespace>/<name>", caSecret)
		}
		s := &corev1.Secret{}
		if err := r.Get(ctx, apitypes.NamespacedName{Namespace: ns, Name: n}, s); err != nil {
			return nil, fmt.Errorf("failed to get secret %s: %w", caSecret, err)
		}
		if ca = s.Data["ca.crt"]; len(ca) == 0 {
			return nil, fmt.Errorf("secret %s has no ca.crt", caSecret)
		}
	}
	transport, err := exportimage.NewTransport(builtinCA, ca)
	if err != nil {
		return nil, err
	}
	stagingKeychain, err := r.stagingKeychain(ctx, stagingPushSecret)
	if err != nil {
		return nil, err
	}
	return &registryCredentials{keychain: keychain, stagingKeychain: stagingKeychain, transport: transport, ca: ca}, nil
}

// stagingKeychain reads the credential limited to the staging project
// ("<namespace>/<name>", a Docker config under config.json or .dockerconfigjson).
func (r *ExportImageJobReconciler) stagingKeychain(ctx context.Context, ref string) (authn.Keychain, error) {
	ns, n, ok := strings.Cut(ref, "/")
	if !ok || ns == "" || n == "" {
		return nil, fmt.Errorf("the staging push secret %q is not <namespace>/<name>", ref)
	}
	s := &corev1.Secret{}
	if err := r.Get(ctx, apitypes.NamespacedName{Namespace: ns, Name: n}, s); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewBadRequest(fmt.Sprintf("%v (secret %s not found); please contact your administrator",
				exportimage.ErrNoStagingCredential, ref))
		}
		return nil, fmt.Errorf("failed to get secret %s: %w", ref, err)
	}
	data, ok := s.Data["config.json"]
	if !ok {
		data, ok = s.Data[corev1.DockerConfigJsonKey]
	}
	if !ok {
		return nil, fmt.Errorf("secret %s has neither config.json nor %s", ref, corev1.DockerConfigJsonKey)
	}
	return exportimage.NewConfigKeychain(data)
}

// generateTargetImageName returns the target image without the registry host:
// <project>/<namespace>/<repository>:<YYYYMMDDHHMMSS>-<suffix>. Two exports of the same
// image in the same second still get different tags, so no export overwrites another.
// Example: "harbor.example.com/proxy/rocm/7.0-preview:tag" -> "custom/rocm/7.0-preview:20250112093000-1a2b3c"
func generateTargetImageName(project, sourceImage string, now time.Time, suffix string) (string, error) {
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
		project,
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
