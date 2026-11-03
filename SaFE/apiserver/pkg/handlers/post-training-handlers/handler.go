/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package posttraininghandlers serves the Post Training API. Post Training is on when the post-training-registry
// ConfigMap exists; otherwise every route answers 404.
package posttraininghandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

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
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	commonworkspace "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workspace"
)

const (
	// defaultStorageRoot is used when a workspace has no volume.
	defaultStorageRoot = "/wekafs"
	// runDirParent is where runs write their directory under the workspace storage.
	runDirParent = "custom/models"
)

// Handler serves the Post Training routes.
type Handler struct {
	client ctrlclient.Client
	db     dbclient.Interface
	access *authority.AccessController

	store    registry.Store
	mu       sync.Mutex
	loadedRV string
}

// NewHandler creates the Post Training handler.
func NewHandler(cli ctrlclient.Client, db dbclient.Interface) *Handler {
	return &Handler{client: cli, db: db, access: authority.NewAccessController(cli)}
}

// registry returns the current registry. A missing ConfigMap means Post Training is off (404); an invalid one keeps
// the previous valid version and, before any valid version was seen, returns the validation error.
func (h *Handler) registry(ctx context.Context) (*registry.Registry, error) {
	cm := &corev1.ConfigMap{}
	key := ctrlclient.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: registry.ConfigMapName}
	if err := h.client.Get(ctx, key, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewNotFoundWithMessage("Post Training is not enabled on this cluster")
		}
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to read the Post Training registry: %v", err))
	}
	h.mu.Lock()
	if cm.ResourceVersion != h.loadedRV {
		prev, _ := h.store.Get()
		check := func(next *registry.Registry) error {
			if err := recipe.CheckRegistry(next); err != nil {
				return err
			}
			return registry.CheckRemovedImages(prev, next, h.imagesInUse(ctx))
		}
		if err := h.store.Load([]byte(cm.Data[registry.DataKey]), check); err != nil {
			klog.ErrorS(err, "rejected a Post Training registry version", "resourceVersion", cm.ResourceVersion)
		}
		h.loadedRV = cm.ResourceVersion
	}
	h.mu.Unlock()
	return h.store.Get()
}

// imagesInUse lists the images of the runs that have not finished.
func (h *Handler) imagesInUse(ctx context.Context) []string {
	if h.db == nil {
		return nil
	}
	runs, _, err := h.db.ListPostTrainingRuns(ctx, dbclient.PostTrainingRunFilter{
		Statuses: []string{string(v1.WorkloadPending), string(v1.WorkloadRunning)}})
	if err != nil {
		klog.ErrorS(err, "failed to list unfinished runs for the registry check")
		return nil
	}
	images := make([]string, 0, len(runs))
	for _, r := range runs {
		images = append(images, r.Image)
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
	ctx := c.Request.Context()
	user, err := h.access.GetRequestUser(ctx, c.GetString(common.UserId))
	if err != nil {
		return err
	}
	roles := h.access.GetRoles(ctx, user)
	kinds := []string{v1.WorkloadKind}
	if verb == v1.CreateVerb {
		kinds = append(kinds, fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(priority)))
	}
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

// getWorkspace reads a workspace and its node flavor.
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
	if info.storageRoot == "" {
		info.storageRoot = defaultStorageRoot
	}
	if ws.Spec.NodeFlavor != "" {
		nf := &v1.NodeFlavor{}
		if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: ws.Spec.NodeFlavor}, nf); err == nil {
			info.flavor = nf
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
	m := &v1.Model{}
	if err := h.client.Get(ctx, ctrlclient.ObjectKey{Name: id}, m); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, recipe.ModelInfo{}, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("model %s not found", id))
		}
		return nil, recipe.ModelInfo{}, err
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
	if err != nil {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("dataset %s not found", id))
	}
	info := &recipe.DatasetInfo{ID: ds.DatasetId, Type: ds.DatasetType}
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
