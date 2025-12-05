package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/tensorboard"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	streamReader *tensorboard.StreamReader
)

// InitStreamReader initializes the stream reader
func InitStreamReader() {
	if tensorboardReader == nil {
		tensorboardReader = tensorboard.NewReader()
	}
	streamReader = tensorboard.NewStreamReader(tensorboardReader)
	log.Info("TensorBoard stream reader initialized")
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: Add proper origin check in production
		return true
	},
}

// StartTensorBoardStream starts streaming TensorBoard logs via WebSocket
// @Summary Start TensorBoard log streaming
// @Description Starts streaming TensorBoard logs via WebSocket connection
// @Tags tensorboard-stream
// @Param workload_uid query string true "Workload UID"
// @Param pod_uid query string true "Pod UID"
// @Param log_dir query string true "Log directory"
// @Param poll_interval query int false "Poll interval in seconds" default(2)
// @Param read_historical query bool false "Read historical data" default(false)
// @Success 101 "Switching Protocols"
// @Router /tensorboard/stream/ws [get]
func StartTensorBoardStream(c *gin.Context) {
	// Parse query parameters
	workloadUID := c.Query("workload_uid")
	podUID := c.Query("pod_uid")
	logDir := c.Query("log_dir")

	if workloadUID == "" || podUID == "" || logDir == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"workload_uid, pod_uid, and log_dir are required",
			nil,
		))
		return
	}

	if streamReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"stream reader not initialized",
			nil,
		))
		return
	}

	// Parse optional parameters
	pollInterval := 2 * time.Second
	if intervalStr := c.Query("poll_interval"); intervalStr != "" {
		if interval, err := time.ParseDuration(intervalStr + "s"); err == nil {
			pollInterval = interval
		}
	}

	readHistorical := c.Query("read_historical") == "true"

	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	log.Infof("WebSocket connection established for workload %s", workloadUID)

	// Start streaming session
	config := &tensorboard.StreamConfig{
		PollInterval:   pollInterval,
		ChunkSize:      64 * 1024,
		BufferSize:     100,
		ReadHistorical: readHistorical,
		FollowRotation: true,
	}

	session, err := streamReader.StartStream(c.Request.Context(), &tensorboard.StreamRequest{
		WorkloadUID: workloadUID,
		PodUID:      podUID,
		LogDir:      logDir,
		Config:      config,
	})

	if err != nil {
		log.Errorf("Failed to start stream: %v", err)
		conn.WriteJSON(map[string]interface{}{
			"type":  "error",
			"error": err.Error(),
		})
		return
	}

	defer func() {
		streamReader.StopStream(workloadUID)
		log.Infof("WebSocket stream stopped for workload %s", workloadUID)
	}()

	// Send initial state
	conn.WriteJSON(map[string]interface{}{
		"type":    "started",
		"message": "Stream started successfully",
		"config":  config,
	})

	// Handle messages from client (for control commands)
	go func() {
		for {
			var msg map[string]interface{}
			err := conn.ReadJSON(&msg)
			if err != nil {
				log.Debugf("WebSocket read error: %v", err)
				session.Stop()
				return
			}

			// Handle control commands
			if cmd, ok := msg["command"].(string); ok {
				switch cmd {
				case "pause":
					// TODO: Implement pause
					log.Info("Stream pause requested")
				case "resume":
					// TODO: Implement resume
					log.Info("Stream resume requested")
				case "get_state":
					state := session.GetState()
					conn.WriteJSON(map[string]interface{}{
						"type":  "state",
						"state": state,
					})
				}
			}
		}
	}()

	// Forward updates to WebSocket
	for {
		select {
		case update, ok := <-session.Updates():
			if !ok {
				// Stream closed
				conn.WriteJSON(map[string]interface{}{
					"type":    "closed",
					"message": "Stream closed",
				})
				return
			}

			// Send update
			err := conn.WriteJSON(map[string]interface{}{
				"type":   "update",
				"update": update,
			})
			if err != nil {
				log.Errorf("Failed to send update via WebSocket: %v", err)
				return
			}

		case err, ok := <-session.Errors():
			if !ok {
				return
			}

			// Send error
			conn.WriteJSON(map[string]interface{}{
				"type":  "error",
				"error": err.Error(),
			})

		case <-c.Request.Context().Done():
			log.Info("Request context cancelled")
			return
		}
	}
}

// StreamTensorBoardSSE streams TensorBoard logs via Server-Sent Events
// @Summary Stream TensorBoard logs (SSE)
// @Description Streams TensorBoard logs via Server-Sent Events
// @Tags tensorboard-stream
// @Accept json
// @Produce text/event-stream
// @Param request body tensorboard.StreamRequest true "Stream request"
// @Success 200 "Event stream"
// @Router /tensorboard/stream/sse [post]
func StreamTensorBoardSSE(c *gin.Context) {
	var req tensorboard.StreamRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if streamReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"stream reader not initialized",
			nil,
		))
		return
	}

	log.Infof("SSE stream requested for workload %s", req.WorkloadUID)

	// Set SSE headers
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	// Start streaming session
	session, err := streamReader.StartStream(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to start stream: %v", err)
		c.SSEvent("error", err.Error())
		c.Writer.Flush()
		return
	}

	defer func() {
		streamReader.StopStream(req.WorkloadUID)
		log.Infof("SSE stream stopped for workload %s", req.WorkloadUID)
	}()

	// Send initial event
	c.SSEvent("started", "Stream started successfully")
	c.Writer.Flush()

	// Forward updates to SSE
	for {
		select {
		case update, ok := <-session.Updates():
			if !ok {
				c.SSEvent("closed", "Stream closed")
				c.Writer.Flush()
				return
			}

			// Marshal update to JSON
			data, err := json.Marshal(update)
			if err != nil {
				log.Errorf("Failed to marshal update: %v", err)
				continue
			}

			// Send as SSE event
			c.SSEvent("update", string(data))
			c.Writer.Flush()

		case err, ok := <-session.Errors():
			if !ok {
				return
			}

			c.SSEvent("error", err.Error())
			c.Writer.Flush()

		case <-c.Request.Context().Done():
			log.Info("SSE client disconnected")
			return
		}
	}
}

// GetStreamState gets the current state of a stream
// @Summary Get stream state
// @Description Gets the current state of an active streaming session
// @Tags tensorboard-stream
// @Param workload_uid path string true "Workload UID"
// @Success 200 {object} rest.Response{data=tensorboard.StreamState}
// @Failure 404 {object} rest.Response
// @Router /tensorboard/stream/{workload_uid}/state [get]
func GetStreamState(c *gin.Context) {
	workloadUID := c.Param("workload_uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"workload_uid is required",
			nil,
		))
		return
	}

	if streamReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"stream reader not initialized",
			nil,
		))
		return
	}

	session, exists := streamReader.GetStream(workloadUID)
	if !exists {
		c.JSON(http.StatusNotFound, rest.ErrorResp(
			c.Request.Context(),
			http.StatusNotFound,
			fmt.Sprintf("no active stream for workload %s", workloadUID),
			nil,
		))
		return
	}

	state := session.GetState()
	c.JSON(http.StatusOK, rest.SuccessResp(c, state))
}

// StopTensorBoardStream stops an active stream
// @Summary Stop stream
// @Description Stops an active streaming session
// @Tags tensorboard-stream
// @Param workload_uid path string true "Workload UID"
// @Success 200 {object} rest.Response
// @Failure 404 {object} rest.Response
// @Router /tensorboard/stream/{workload_uid}/stop [post]
func StopTensorBoardStream(c *gin.Context) {
	workloadUID := c.Param("workload_uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"workload_uid is required",
			nil,
		))
		return
	}

	if streamReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"stream reader not initialized",
			nil,
		))
		return
	}

	err := streamReader.StopStream(workloadUID)
	if err != nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(
			c.Request.Context(),
			http.StatusNotFound,
			err.Error(),
			nil,
		))
		return
	}

	log.Infof("Stream stopped for workload %s", workloadUID)

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"workload_uid": workloadUID,
		"stopped":      true,
	}))
}

// ResumeStream resumes a stream from saved state
// @Summary Resume stream
// @Description Resumes streaming from a previously saved state
// @Tags tensorboard-stream
// @Accept json
// @Produce json
// @Param request body tensorboard.StreamRequest true "Stream request with resume state"
// @Success 200 {object} rest.Response
// @Router /tensorboard/stream/resume [post]
func ResumeStream(c *gin.Context) {
	var req tensorboard.StreamRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"invalid request: "+err.Error(),
			nil,
		))
		return
	}

	if req.ResumeState == nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			c.Request.Context(),
			http.StatusBadRequest,
			"resume_state is required",
			nil,
		))
		return
	}

	if streamReader == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"stream reader not initialized",
			nil,
		))
		return
	}

	log.Infof("Resuming stream for workload %s from state %s",
		req.WorkloadUID, req.ResumeState.SessionID)

	session, err := streamReader.StartStream(c.Request.Context(), &req)
	if err != nil {
		log.Errorf("Failed to resume stream: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			c.Request.Context(),
			http.StatusInternalServerError,
			"failed to resume stream: "+err.Error(),
			nil,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"workload_uid": req.WorkloadUID,
		"resumed":      true,
		"state":        session.GetState(),
	}))
}
