package api

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	processtree "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/process-tree"
	"github.com/gin-gonic/gin"
)

// GetPodProcessTree retrieves the complete process tree for a pod
func GetPodProcessTree(c *gin.Context) {
	var req processtree.ProcessTreeRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, err.Error(), nil))
		return
	}

	collector := processtree.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "process tree collector not initialized", nil))
		return
	}

	tree, err := collector.GetPodProcessTree(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, tree))
}

// FindPythonProcessesInPod finds all Python processes in a pod
func FindPythonProcessesInPod(c *gin.Context) {
	var req struct {
		PodUID string `json:"pod_uid" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, err.Error(), nil))
		return
	}

	collector := processtree.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "process tree collector not initialized", nil))
		return
	}

	processes, err := collector.FindPythonProcesses(c.Request.Context(), req.PodUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, processes))
}

// FindTensorboardFilesInPod finds all tensorboard event files opened by processes in a pod
func FindTensorboardFilesInPod(c *gin.Context) {
	var req struct {
		PodUID       string `json:"pod_uid" binding:"required"`
		PodName      string `json:"pod_name"`
		PodNamespace string `json:"pod_namespace"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, err.Error(), nil))
		return
	}

	collector := processtree.GetCollector()
	if collector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "process tree collector not initialized", nil))
		return
	}

	tensorboardFiles, err := collector.FindTensorboardFiles(c.Request.Context(), req.PodUID, req.PodName, req.PodNamespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, tensorboardFiles))
}
