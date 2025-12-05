package handlers

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/tensorboard"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

var tensorboardReader *tensorboard.Reader

// InitTensorBoardReader initializes the TensorBoard reader
func InitTensorBoardReader() {
	tensorboardReader = tensorboard.NewReader()
	log.Info("TensorBoard reader initialized")
}

// GetTensorBoardLogs retrieves TensorBoard log files for a workload
// @Summary Get TensorBoard logs
// @Description Retrieves TensorBoard event files from training container (non-intrusive)
// @Tags tensorboard
// @Accept json
// @Produce json
// @Param request body tensorboard.LogReadRequest true "Log read request"
// @Success 200 {object} rest.Response{data=tensorboard.LogReadResponse}
// @Failure 400 {object} rest.Response
// @Failure 500 {object} rest.Response
// @Router /tensorboard/logs [post]
func GetTensorBoardLogs(c *gin.Context) {
	var req tensorboard.LogReadRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if tensorboardReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"tensorboard reader not initialized",
			nil,
		))
		return
	}

	log.Infof("Getting TensorBoard logs for workload %s, log_dir=%s",
		req.WorkloadUID, req.LogDir)

	response, err := tensorboardReader.ReadLogs(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to get TensorBoard logs: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to get TensorBoard logs: "+err.Error(),
			nil,
		))
		return
	}

	log.Infof("Successfully retrieved %d TensorBoard event files, total size: %d bytes",
		response.FileCount, response.TotalSize)

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// ReadTensorBoardEvent reads a specific TensorBoard event file
// @Summary Read TensorBoard event file
// @Description Reads content from a TensorBoard event file (non-intrusive)
// @Tags tensorboard
// @Accept json
// @Produce json
// @Param request body tensorboard.EventReadRequest true "Event read request"
// @Success 200 {object} rest.Response{data=tensorboard.EventReadResponse}
// @Failure 400 {object} rest.Response
// @Failure 500 {object} rest.Response
// @Router /tensorboard/event [post]
func ReadTensorBoardEvent(c *gin.Context) {
	var req tensorboard.EventReadRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if tensorboardReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"tensorboard reader not initialized",
			nil,
		))
		return
	}

	log.Infof("Reading TensorBoard event file: workload=%s, file=%s, offset=%d, length=%d",
		req.WorkloadUID, req.EventFile, req.Offset, req.Length)

	response, err := tensorboardReader.ReadEvent(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to read event file: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to read event file: "+err.Error(),
			nil,
		))
		return
	}

	log.Debugf("Successfully read %d bytes from event file", response.BytesRead)

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// ListTensorBoardEventFiles lists all TensorBoard event files
// @Summary List TensorBoard event files
// @Description Lists all event files in TensorBoard log directory
// @Tags tensorboard
// @Accept json
// @Produce json
// @Param request body tensorboard.LogReadRequest true "List request"
// @Success 200 {object} rest.Response{data=[]types.ContainerFileInfo}
// @Failure 400 {object} rest.Response
// @Failure 500 {object} rest.Response
// @Router /tensorboard/files [post]
func ListTensorBoardEventFiles(c *gin.Context) {
	var req tensorboard.LogReadRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if tensorboardReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"tensorboard reader not initialized",
			nil,
		))
		return
	}

	log.Infof("Listing TensorBoard event files for workload %s, log_dir=%s",
		req.WorkloadUID, req.LogDir)

	files, err := tensorboardReader.ListEventFiles(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to list event files: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to list event files: "+err.Error(),
			nil,
		))
		return
	}

	log.Infof("Found %d event files", len(files))

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"files": files,
		"total": len(files),
	}))
}

// ReadContainerFile reads any file from training container
// @Summary Read container file
// @Description Reads a file from training container filesystem (non-intrusive, with security restrictions)
// @Tags tensorboard
// @Accept json
// @Produce json
// @Param request body object{pod_uid=string,file_path=string,offset=int64,length=int64} true "File read request"
// @Success 200 {object} rest.Response{data=types.ContainerFileReadResponse}
// @Failure 400 {object} rest.Response
// @Failure 500 {object} rest.Response
// @Router /tensorboard/file/read [post]
func ReadContainerFile(c *gin.Context) {
	var req struct {
		PodUID   string `json:"pod_uid" binding:"required"`
		FilePath string `json:"file_path" binding:"required"`
		Offset   int64  `json:"offset,omitempty"`
		Length   int64  `json:"length,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if tensorboardReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"tensorboard reader not initialized",
			nil,
		))
		return
	}

	log.Infof("Reading container file: pod=%s, path=%s", req.PodUID, req.FilePath)

	response, err := tensorboardReader.ReadFile(
		c.Request.Context(), req.PodUID, req.FilePath, req.Offset, req.Length,
	)
	if err != nil {
		log.Errorf("Failed to read container file: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to read file: "+err.Error(),
			nil,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// GetContainerFileInfo gets metadata for a file in container
// @Summary Get container file info
// @Description Gets metadata for a file in training container
// @Tags tensorboard
// @Accept json
// @Produce json
// @Param request body object{pod_uid=string,file_path=string} true "File info request"
// @Success 200 {object} rest.Response{data=types.ContainerFileInfo}
// @Failure 400 {object} rest.Response
// @Failure 500 {object} rest.Response
// @Router /tensorboard/file/info [post]
func GetContainerFileInfo(c *gin.Context) {
	var req struct {
		PodUID   string `json:"pod_uid" binding:"required"`
		FilePath string `json:"file_path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if tensorboardReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"tensorboard reader not initialized",
			nil,
		))
		return
	}

	log.Debugf("Getting container file info: pod=%s, path=%s", req.PodUID, req.FilePath)

	fileInfo, err := tensorboardReader.GetFileInfo(c.Request.Context(), req.PodUID, req.FilePath)
	if err != nil {
		log.Errorf("Failed to get file info: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to get file info: "+err.Error(),
			nil,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, fileInfo))
}

