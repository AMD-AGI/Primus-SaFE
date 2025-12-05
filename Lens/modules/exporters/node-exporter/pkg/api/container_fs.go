package api

import (
	"net/http"

	containerfs "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/container-fs"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

var (
	fsReader           *containerfs.FSReader
	tensorboardReader  *containerfs.TensorBoardReader
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
		c.JSON(http.StatusBadRequest, Response{
			Code: http.StatusBadRequest,
			Msg:  "Invalid request: " + err.Error(),
		})
		return
	}

	log.Infof("Reading container file: pid=%d, path=%s, offset=%d, length=%d",
		req.PID, req.Path, req.Offset, req.Length)

	response, err := fsReader.ReadFile(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to read container file: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to read file: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: response,
		Msg:  "success",
	})
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
		c.JSON(http.StatusBadRequest, Response{
			Code: http.StatusBadRequest,
			Msg:  "Invalid request: " + err.Error(),
		})
		return
	}

	log.Infof("Listing container directory: pid=%d, path=%s, recursive=%v",
		req.PID, req.Path, req.Recursive)

	response, err := fsReader.ListDirectory(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to list container directory: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to list directory: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: response,
		Msg:  "success",
	})
}

// GetContainerFileInfo gets file metadata from container
// @Summary Get file info from container
// @Description Gets file metadata from container's filesystem
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body object{pid=int,path=string} true "File info request"
// @Success 200 {object} Response{data=containerfs.FileInfo}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/info [post]
func GetContainerFileInfo(c *gin.Context) {
	var req struct {
		PID  int    `json:"pid" binding:"required"`
		Path string `json:"path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: http.StatusBadRequest,
			Msg:  "Invalid request: " + err.Error(),
		})
		return
	}

	log.Infof("Getting container file info: pid=%d, path=%s", req.PID, req.Path)

	fileInfo, err := fsReader.GetFileInfo(c.Request.Context(), req.PID, req.Path)
	if err != nil {
		log.Errorf("Failed to get container file info: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to get file info: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: fileInfo,
		Msg:  "success",
	})
}

// GetTensorBoardLogs retrieves TensorBoard event files from container
// @Summary Get TensorBoard logs
// @Description Retrieves TensorBoard event files from container's log directory
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body object{pid=int,log_dir=string} true "TensorBoard log request"
// @Success 200 {object} Response{data=containerfs.TensorBoardLogInfo}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/tensorboard/logs [post]
func GetTensorBoardLogs(c *gin.Context) {
	var req struct {
		PID    int    `json:"pid" binding:"required"`
		LogDir string `json:"log_dir" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: http.StatusBadRequest,
			Msg:  "Invalid request: " + err.Error(),
		})
		return
	}

	log.Infof("Getting TensorBoard logs: pid=%d, log_dir=%s", req.PID, req.LogDir)

	logInfo, err := tensorboardReader.GetTensorBoardLogs(c.Request.Context(), req.PID, req.LogDir)
	if err != nil {
		log.Errorf("Failed to get TensorBoard logs: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to get TensorBoard logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: logInfo,
		Msg:  "success",
	})
}

// ReadTensorBoardEvent reads a TensorBoard event file
// @Summary Read TensorBoard event file
// @Description Reads a specific TensorBoard event file from container
// @Tags container-fs
// @Accept json
// @Produce json
// @Param request body object{pid=int,event_file=string,offset=int64,length=int64} true "Event read request"
// @Success 200 {object} Response{data=containerfs.ReadResponse}
// @Failure 400 {object} Response
// @Failure 500 {object} Response
// @Router /container-fs/tensorboard/event [post]
func ReadTensorBoardEvent(c *gin.Context) {
	var req struct {
		PID       int    `json:"pid" binding:"required"`
		EventFile string `json:"event_file" binding:"required"`
		Offset    int64  `json:"offset,omitempty"`
		Length    int64  `json:"length,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code: http.StatusBadRequest,
			Msg:  "Invalid request: " + err.Error(),
		})
		return
	}

	log.Infof("Reading TensorBoard event: pid=%d, file=%s, offset=%d, length=%d",
		req.PID, req.EventFile, req.Offset, req.Length)

	response, err := tensorboardReader.ReadTensorBoardEvent(
		c.Request.Context(), req.PID, req.EventFile, req.Offset, req.Length,
	)
	if err != nil {
		log.Errorf("Failed to read TensorBoard event: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code: http.StatusInternalServerError,
			Msg:  "Failed to read event file: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code: 0,
		Data: response,
		Msg:  "success",
	})
}

