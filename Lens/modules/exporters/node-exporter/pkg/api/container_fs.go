package api

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	containerfs "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/container-fs"
	"github.com/gin-gonic/gin"
)

var (
	fsReader          *containerfs.FSReader
	tensorboardReader *containerfs.TensorBoardReader
)

// InitContainerFS initializes container filesystem readers
func InitContainerFS() {
	fsReader = containerfs.NewFSReader()
	tensorboardReader = containerfs.NewTensorBoardReader()
	log.Info("Container filesystem readers initialized")
}

// ReadContainerFile reads a file from container filesystem
// @Summary Read file from container
// @Description Reads a file from container's filesystem via /proc/[pid]/root
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body containerfs.ReadRequest true "Read request"
// @Success 200 {object} Response{data=containerfs.ReadResponse}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/read [post]
func ReadContainerFile(c *gin.Context) {
	var req containerfs.ReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"Invalid request: "+err.Error(),
			err,
		))
		return
	}

	if req.PID > 0 {
		log.Infof("Reading container file: pid=%d, path=%s, offset=%d, length=%d",
			req.PID, req.Path, req.Offset, req.Length)
	} else {
		log.Infof("Reading container file: pod_uid=%s, container=%s, path=%s, offset=%d, length=%d",
			req.PodUID, req.ContainerName, req.Path, req.Offset, req.Length)
	}

	response, err := fsReader.ReadFile(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to read container file: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to read file: "+err.Error(),
			err,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// ListContainerDirectory lists files in a container directory
// @Summary List directory from container
// @Description Lists files in a directory from container's filesystem
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body containerfs.ListRequest true "List request"
// @Success 200 {object} Response{data=containerfs.ListResponse}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/list [post]
func ListContainerDirectory(c *gin.Context) {
	var req containerfs.ListRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"Invalid request: "+err.Error(),
			err,
		))
		return
	}

	if req.PID > 0 {
		log.Infof("Listing container directory: pid=%d, path=%s, recursive=%v",
			req.PID, req.Path, req.Recursive)
	} else {
		log.Infof("Listing container directory: pod_uid=%s, container=%s, path=%s, recursive=%v",
			req.PodUID, req.ContainerName, req.Path, req.Recursive)
	}

	response, err := fsReader.ListDirectory(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to list container directory: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to list directory: "+err.Error(),
			err,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// GetContainerFileInfo gets file metadata from container
// @Summary Get file info from container
// @Description Gets file metadata from container's filesystem
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body object{pid=int,pod_uid=string,pod_name=string,pod_namespace=string,container_name=string,path=string} true "File info request"
// @Success 200 {object} Response{data=containerfs.FileInfo}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/info [post]
func GetContainerFileInfo(c *gin.Context) {
	var req struct {
		// Option 1: Specify PID directly (highest priority)
		PID int `json:"pid,omitempty"`

		// Option 2: Specify Pod (will auto-select first process in main container)
		PodUID        string `json:"pod_uid,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodNamespace  string `json:"pod_namespace,omitempty"`
		ContainerName string `json:"container_name,omitempty"`

		Path string `json:"path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"Invalid request: "+err.Error(),
			err,
		))
		return
	}

	if req.PID > 0 {
		log.Infof("Getting container file info: pid=%d, path=%s", req.PID, req.Path)
	} else {
		log.Infof("Getting container file info: pod_uid=%s, container=%s, path=%s",
			req.PodUID, req.ContainerName, req.Path)
	}

	// Resolve PID if pod parameters are provided
	pid, err := fsReader.ResolvePID(c.Request.Context(), req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		log.Errorf("Failed to resolve PID: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to resolve PID: "+err.Error(),
			err,
		))
		return
	}

	fileInfo, err := fsReader.GetFileInfo(c.Request.Context(), pid, req.Path)
	if err != nil {
		log.Errorf("Failed to get container file info: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to get file info: "+err.Error(),
			err,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, fileInfo))
}

// GetTensorBoardLogs retrieves TensorBoard event files from container
// @Summary Get TensorBoard logs
// @Description Retrieves TensorBoard event files from container's log directory
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body object{pid=int,pod_uid=string,pod_name=string,pod_namespace=string,container_name=string,log_dir=string} true "TensorBoard log request"
// @Success 200 {object} Response{data=containerfs.TensorBoardLogInfo}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/tensorboard/logs [post]
func GetTensorBoardLogs(c *gin.Context) {
	var req struct {
		// Option 1: Specify PID directly (highest priority)
		PID int `json:"pid,omitempty"`

		// Option 2: Specify Pod (will auto-select first process in main container)
		PodUID        string `json:"pod_uid,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodNamespace  string `json:"pod_namespace,omitempty"`
		ContainerName string `json:"container_name,omitempty"`

		LogDir string `json:"log_dir" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"Invalid request: "+err.Error(),
			err,
		))
		return
	}

	if req.PID > 0 {
		log.Infof("Getting TensorBoard logs: pid=%d, log_dir=%s", req.PID, req.LogDir)
	} else {
		log.Infof("Getting TensorBoard logs: pod_uid=%s, container=%s, log_dir=%s",
			req.PodUID, req.ContainerName, req.LogDir)
	}

	// Resolve PID if pod parameters are provided
	pid, err := fsReader.ResolvePID(c.Request.Context(), req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		log.Errorf("Failed to resolve PID: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to resolve PID: "+err.Error(),
			err,
		))
		return
	}

	logInfo, err := tensorboardReader.GetTensorBoardLogs(c.Request.Context(), pid, req.LogDir)
	if err != nil {
		log.Errorf("Failed to get TensorBoard logs: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to get TensorBoard logs: "+err.Error(),
			err,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, logInfo))
}

// ReadTensorBoardEvent reads a TensorBoard event file
// @Summary Read TensorBoard event file
// @Description Reads a specific TensorBoard event file from container
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body object{pid=int,pod_uid=string,pod_name=string,pod_namespace=string,container_name=string,event_file=string,offset=int64,length=int64} true "Event read request"
// @Success 200 {object} Response{data=containerfs.ReadResponse}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/tensorboard/event [post]
func ReadTensorBoardEvent(c *gin.Context) {
	var req struct {
		// Option 1: Specify PID directly (highest priority)
		PID int `json:"pid,omitempty"`

		// Option 2: Specify Pod (will auto-select first process in main container)
		PodUID        string `json:"pod_uid,omitempty"`
		PodName       string `json:"pod_name,omitempty"`
		PodNamespace  string `json:"pod_namespace,omitempty"`
		ContainerName string `json:"container_name,omitempty"`

		EventFile string `json:"event_file" binding:"required"`
		Offset    int64  `json:"offset,omitempty"`
		Length    int64  `json:"length,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"Invalid request: "+err.Error(),
			err,
		))
		return
	}

	if req.PID > 0 {
		log.Infof("Reading TensorBoard event: pid=%d, file=%s, offset=%d, length=%d",
			req.PID, req.EventFile, req.Offset, req.Length)
	} else {
		log.Infof("Reading TensorBoard event: pod_uid=%s, container=%s, file=%s, offset=%d, length=%d",
			req.PodUID, req.ContainerName, req.EventFile, req.Offset, req.Length)
	}

	// Resolve PID if pod parameters are provided
	pid, err := fsReader.ResolvePID(c.Request.Context(), req.PID, req.PodUID, req.PodName, req.PodNamespace, req.ContainerName)
	if err != nil {
		log.Errorf("Failed to resolve PID: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to resolve PID: "+err.Error(),
			err,
		))
		return
	}

	response, err := tensorboardReader.ReadTensorBoardEvent(
		c.Request.Context(), pid, req.EventFile, req.Offset, req.Length,
	)
	if err != nil {
		log.Errorf("Failed to read TensorBoard event: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"Failed to read event file: "+err.Error(),
			err,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}
