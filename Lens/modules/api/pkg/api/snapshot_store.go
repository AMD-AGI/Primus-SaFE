// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"net/http"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/snapshot"
	"github.com/gin-gonic/gin"
)

var (
	globalSnapshotStore snapshot.Store
	snapshotStoreOnce   sync.Once
)

// InitSnapshotStore initializes the package-level snapshot store from config.
// Safe to call multiple times; only the first call takes effect.
func InitSnapshotStore(cfg *config.Config) {
	snapshotStoreOnce.Do(func() {
		if cfg.SnapshotStore == nil || !cfg.SnapshotStore.Enabled {
			log.Info("Snapshot store not configured, code snapshot download will be unavailable")
			return
		}
		snapCfg := cfg.SnapshotStore.ToSnapshotConfig()
		store, err := snapshot.New(snapCfg)
		if err != nil {
			log.Errorf("Failed to initialize snapshot store (%s): %v", snapCfg.Type, err)
			return
		}
		globalSnapshotStore = store
		log.Infof("Snapshot store initialized: type=%s", snapCfg.Type)
	})
}

func init() {
	unified.Register(&unified.EndpointDef[struct{}, struct{}]{
		Name:           "workload_diag_code_snapshot_download",
		Description:    "Download workload code snapshot source files as a tar.gz archive",
		Group:          "diagnostic",
		HTTPMethod:     "GET",
		HTTPPath:       "/workload-diag/:uid/code-snapshot-download",
		MCPOnly:        false,
		HTTPOnly:       true,
		RawHTTPHandler: unified.RawHTTPHandler(handleCodeSnapshotDownload),
	})
}

func handleCodeSnapshotDownload(c *gin.Context) {
	uid := c.Param("uid")
	cluster := c.Query("cluster")

	if uid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uid is required"})
		return
	}

	store := globalSnapshotStore
	if store == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "snapshot store not configured"})
		return
	}

	ctx := c.Request.Context()

	clusterName, err := ResolveWorkloadCluster(ctx, uid, cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	facade := database.GetFacadeForCluster(clusterName)
	record, err := facade.GetWorkloadCodeSnapshot().GetByWorkloadUID(ctx, uid)
	if err != nil {
		_ = c.Error(errors.NewError().WithCode(errors.InternalError).WithMessage("failed to get code snapshot: " + err.Error()))
		return
	}
	if record == nil {
		_ = c.Error(errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("no code snapshot found for workload: " + uid))
		return
	}
	if record.StorageKey == nil || *record.StorageKey == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "snapshot has no external storage (content stored inline in DB, download not available)"})
		return
	}

	files, err := store.Load(ctx, *record.StorageKey)
	if err != nil {
		log.Warnf("Failed to load snapshot from store key=%s: %v", *record.StorageKey, err)
		_ = c.Error(errors.NewError().WithCode(errors.InternalError).WithMessage("failed to load snapshot from store"))
		return
	}
	if len(files) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no files found in snapshot store"})
		return
	}

	filename := fmt.Sprintf("code-snapshot-%s.tar.gz", record.Fingerprint)
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	gw := gzip.NewWriter(c.Writer)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	for _, f := range files {
		hdr := &tar.Header{
			Name: f.RelPath,
			Mode: 0644,
			Size: int64(len(f.Content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Warnf("tar write header error for %s: %v", f.RelPath, err)
			return
		}
		if _, err := tw.Write(f.Content); err != nil {
			log.Warnf("tar write content error for %s: %v", f.RelPath, err)
			return
		}
	}
}
