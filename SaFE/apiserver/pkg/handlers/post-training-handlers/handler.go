/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package posttraininghandlers serves the Post Training API. Its routes are registered when Post Training is enabled
// in the apiserver config and the database is; they then answer 404 until the post-training-registry ConfigMap
// exists.
package posttraininghandlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe"
	_ "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/post-training-handlers/recipe/plugins"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/posttraining/registry"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

const (
	// runDirParent is where runs write their directory under the workspace storage.
	runDirParent = "custom/models"
	// registryRetryInterval is how often a version rejected for images in use, or for not knowing them, is checked
	// again.
	registryRetryInterval = time.Minute
	imagesInUseTimeout    = 10 * time.Second
	// detachedTimeout bounds what a handler does apart from its request, such as undoing what it started.
	detachedTimeout = time.Minute
)

// errImagesInUseUnknown rejects a registry version while the images of the unfinished runs cannot be read.
var errImagesInUseUnknown = errors.New("invalid registry: the images that unfinished runs use cannot be read")

// Handler serves the Post Training routes.
type Handler struct {
	client ctrlclient.Client
	db     dbclient.Interface
	access *authority.AccessController

	store    registry.Store
	mu       sync.Mutex
	loadedRV string
	retryAt  time.Time
}

// NewHandler creates the Post Training handler.
func NewHandler(cli ctrlclient.Client, db dbclient.Interface) *Handler {
	return &Handler{client: cli, db: db, access: authority.NewAccessController(cli)}
}

// detached returns a context for work that has to finish although its request ended, for at most detachedTimeout.
func detached(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), detachedTimeout)
}

// registry returns the current registry. A missing ConfigMap means Post Training is off (404); an invalid one keeps
// the previous valid version and, before any valid version was seen, returns the validation error. A version rejected
// for images that unfinished runs use is checked again every registryRetryInterval; the first version after a restart
// is not rejected for them.
func (h *Handler) registry(ctx context.Context) (*registry.Registry, error) {
	cm := &corev1.ConfigMap{}
	key := ctrlclient.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: registry.ConfigMapName}
	if err := h.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFoundWithMessage("Post Training is not enabled on this cluster")
		}
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read the Post Training registry: %v", err))
	}
	// stale tells, under h.mu, whether the ConfigMap holds a version to load.
	stale := func() bool {
		return cm.ResourceVersion != h.loadedRV || (!h.retryAt.IsZero() && time.Now().After(h.retryAt))
	}
	h.mu.Lock()
	reload := stale()
	h.mu.Unlock()
	if !reload {
		return h.store.Get()
	}
	// The unfinished runs are read before the lock is taken, as the query may take up to imagesInUseTimeout and
	// every Post Training request reads the registry.
	runs, runsErr := h.unfinishedRuns()
	h.mu.Lock()
	// Another request may have loaded this version meanwhile.
	if stale() {
		prev, _ := h.store.Get()
		check := func(next *registry.Registry) error {
			if err := recipe.CheckRegistry(next); err != nil {
				return err
			}
			var err error
			if runsErr != nil {
				err = fmt.Errorf("%w: %v", errImagesInUseUnknown, runsErr)
			} else {
				err = registry.CheckRemovedImages(prev, next, imagesInUse(runs, prev != nil))
			}
			if err != nil && prev == nil {
				// After a restart there is no earlier version to keep serving, so the first one is taken as it is.
				klog.Warningf("the first Post Training registry version is taken although: %v", err)
				return nil
			}
			return err
		}
		err := h.store.Load([]byte(cm.Data[registry.DataKey]), check)
		h.loadedRV, h.retryAt = cm.ResourceVersion, time.Time{}
		if err != nil {
			klog.ErrorS(err, "rejected a Post Training registry version", "resourceVersion", cm.ResourceVersion)
			if errors.Is(err, registry.ErrRemovedImagesInUse) || errors.Is(err, errImagesInUseUnknown) {
				h.retryAt = time.Now().Add(registryRetryInterval)
			}
		}
	}
	h.mu.Unlock()
	return h.store.Get()
}

// unfinishedRuns lists the runs that have not finished. The query has its own deadline, as the request that loads
// the registry may end first and its result decides the registry every request then uses.
func (h *Handler) unfinishedRuns() ([]*dbclient.PostTrainingRun, error) {
	if h.db == nil {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), imagesInUseTimeout)
	defer cancel()
	runs, _, err := h.db.ListPostTrainingRuns(ctx, dbclient.PostTrainingRunFilter{
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}})
	return runs, err
}

// imagesInUse lists the images of unfinished runs. Unless all is set, only the runs whose recipe records an image
// from the image table count, as a custom image or an older recipe cannot be told apart from a removed image without
// the previous registry version.
func imagesInUse(runs []*dbclient.PostTrainingRun, all bool) []string {
	images := make([]string, 0, len(runs))
	for _, r := range runs {
		var rec recipe.Recipe
		if all || (json.Unmarshal([]byte(r.Recipe), &rec) == nil && rec.Provenance.TableImage) {
			images = append(images, r.Image)
		}
	}
	return images
}

// requireEnabled answers 404 when Post Training is off, and returns the registry or its load error otherwise.
func (h *Handler) requireEnabled(c *gin.Context) (*registry.Registry, error, bool) {
	reg, err := h.registry(c.Request.Context())
	if commonerrors.IsNotFound(err) {
		apiutils.AbortWithApiError(c, err)
		return nil, nil, false
	}
	return reg, err, true
}

// authorize checks a workload action in a workspace the way direct workload creation does; owner is empty for
// create.
func (h *Handler) authorize(c *gin.Context, verb v1.RoleVerb, workspace, owner, name string, priority int) error {
	kinds := []string{v1.WorkloadKind}
	if verb == v1.CreateVerb {
		kinds = append(kinds, fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(priority)))
	}
	return h.authorizeKinds(c, verb, workspace, owner, name, kinds...)
}

// authorizeKinds checks that the requesting user may act with verb on each resource kind in a workspace.
func (h *Handler) authorizeKinds(c *gin.Context, verb v1.RoleVerb, workspace, owner, name string,
	kinds ...string) error {
	ctx := c.Request.Context()
	user, err := h.access.GetRequestUser(ctx, c.GetString(common.UserId))
	if err != nil {
		return err
	}
	roles := h.access.GetRoles(ctx, user)
	for _, kind := range kinds {
		if err := h.access.Authorize(authority.AccessInput{
			Context:       ctx,
			ResourceKind:  kind,
			ResourceOwner: owner,
			ResourceName:  name,
			Verb:          verb,
			Workspaces:    []string{workspace},
			User:          user,
			UserId:        c.GetString(common.UserId),
			Roles:         roles,
		}); err != nil {
			return err
		}
	}
	return nil
}

// workspaceInfo describes a workspace for the compiler: its cluster, GPU model, node count, per-node allocatable
// resources and storage root.
type workspaceInfo struct {
	recipe.WorkspaceInfo
	flavor      *v1.NodeFlavor
	storageRoot string
}

// getWorkspace reads a workspace and its node flavor. storageRoot is empty for a workspace without a volume, which
// runs cannot write to.
func (h *Handler) getWorkspace(ctx context.Context, name string) (*workspaceInfo, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("workspace is required")
	}
	ws := &v1.Workspace{}
	if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: name}, ws); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("workspace %s not found", name))
		}
		return nil, err
	}
	info := &workspaceInfo{
		WorkspaceInfo: recipe.WorkspaceInfo{ID: ws.Name, Cluster: ws.Spec.Cluster, Nodes: ws.Status.AvailableReplica},
		storageRoot:   commonworkspace.GetNfsPathFromWorkspace(ws),
	}
	if ws.Spec.NodeFlavor != "" {
		nf := &v1.NodeFlavor{}
		if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: ws.Spec.NodeFlavor}, nf); err == nil {
			info.flavor = nf
			if q, ok := quantity.GetAvailableResource(nf.ToResourceList(""))[corev1.ResourceMemory]; ok {
				info.HostMemoryBytes = q.Value()
			}
			info.NodeGPUs = nf.GetGpuCount()
			if nf.Spec.Gpu != nil {
				info.GPUModel = string(nf.Spec.Gpu.Product)
			}
		} else if !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	return info, nil
}

// runDir is the directory a run writes on the workspace storage.
func (w *workspaceInfo) runDir(runID string) string {
	return fmt.Sprintf("%s/%s/%s", w.storageRoot, runDirParent, runID)
}

// getModel reads a model and what the compiler needs to know about it in a workspace.
func (h *Handler) getModel(ctx context.Context, id, workspace string) (*v1.Model, recipe.ModelInfo, error) {
	if id == "" {
		return nil, recipe.ModelInfo{}, commonerrors.NewBadRequest("modelId is required")
	}
	m := &v1.Model{}
	if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: id}, m); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, recipe.ModelInfo{}, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("model %s not found", id))
		}
		return nil, recipe.ModelInfo{}, err
	}
	// The model controller removes the weights of a deleted model, which a run would then lose midway.
	if !m.DeletionTimestamp.IsZero() {
		return nil, recipe.ModelInfo{}, commonerrors.NewBadRequest(fmt.Sprintf("model %s is being deleted", id))
	}
	return m, modelInfo(m, workspace), nil
}

// modelInfo summarizes a model for the compiler: it is ready in a workspace when it is a local model whose copy in
// that workspace is ready.
func modelInfo(m *v1.Model, workspace string) recipe.ModelInfo {
	info := recipe.ModelInfo{ID: m.Name, Config: m.Status.ModelConfig, MaxTokens: m.Spec.MaxTokens}
	if m.Annotations != nil {
		info.ConfigError = m.Annotations[v1.ModelInspectErrorAnnotation]
	}
	if m.IsRemoteAPI() || !m.IsReady() {
		return info
	}
	if path := modelPathIn(m, workspace); path != "" {
		info.Path, info.Ready = path, true
	}
	return info
}

// modelPathIn returns the ready local path of a model in a workspace, or "".
func modelPathIn(m *v1.Model, workspace string) string {
	if m.Spec.Source.AccessMode == v1.AccessModeLocalPath && m.Spec.Source.LocalPath != "" &&
		(m.Spec.Workspace == "" || m.Spec.Workspace == workspace) {
		for _, lp := range m.Status.LocalPaths {
			if lp.Workspace == workspace && lp.Status == v1.LocalPathStatusReady {
				return lp.Path
			}
		}
		return m.Spec.Source.LocalPath
	}
	for _, lp := range m.Status.LocalPaths {
		if lp.Workspace == workspace && lp.Status == v1.LocalPathStatusReady {
			return lp.Path
		}
	}
	return ""
}

// getDataset reads a dataset and where it is in a workspace; nil when id is empty.
func (h *Handler) getDataset(ctx context.Context, id, workspace string) (*recipe.DatasetInfo, error) {
	if id == "" {
		return nil, nil
	}
	ds, err := h.db.GetDataset(ctx, id)
	if datasetMissing(err) {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("dataset %s not found", id))
	}
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read dataset %s: %v", id, err))
	}
	info := &recipe.DatasetInfo{ID: ds.DatasetId, Name: ds.DisplayName, Type: ds.DatasetType}
	var paths []dbclient.DatasetLocalPathDB
	if ds.LocalPaths != "" {
		if err := json.Unmarshal([]byte(ds.LocalPaths), &paths); err != nil {
			klog.ErrorS(err, "invalid dataset local paths", "dataset", id)
		}
	}
	for _, p := range paths {
		if p.Workspace == workspace && p.Status == dbclient.DatasetStatusReady {
			info.Path, info.Ready = p.Path, true
		}
	}
	return info, nil
}
