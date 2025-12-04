package api

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	pythoninspector "github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/python-inspector"
	"github.com/gin-gonic/gin"
)

// ListAvailableScripts lists all available inspection scripts
func ListAvailableScripts(c *gin.Context) {
	inspector := pythoninspector.GetInspector()
	if inspector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "inspector not initialized", nil))
		return
	}

	scriptManager := inspector.GetScriptManager()
	scripts := scriptManager.ListEnabledScripts()

	// Build response
	response := make([]map[string]interface{}, 0, len(scripts))
	for _, script := range scripts {
		response = append(response, map[string]interface{}{
			"name":         script.Metadata.Name,
			"version":      script.Metadata.Version,
			"description":  script.Metadata.Description,
			"tags":         script.Metadata.Tags,
			"timeout":      script.Metadata.Timeout,
			"safety_level": script.Metadata.SafetyLevel,
		})
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// GetScriptDetail returns detailed information about a script
func GetScriptDetail(c *gin.Context) {
	scriptName := c.Param("name")

	inspector := pythoninspector.GetInspector()
	if inspector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "inspector not initialized", nil))
		return
	}

	scriptManager := inspector.GetScriptManager()
	script, err := scriptManager.GetScript(scriptName)
	if err != nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c, http.StatusNotFound, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, script.Metadata))
}

// SearchScripts searches for scripts
func SearchScripts(c *gin.Context) {
	query := c.Query("q")
	tag := c.Query("tag")

	inspector := pythoninspector.GetInspector()
	if inspector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "inspector not initialized", nil))
		return
	}

	scriptManager := inspector.GetScriptManager()

	var scripts []*pythoninspector.InspectionScript
	if tag != "" {
		scripts = scriptManager.GetScriptsByTag(tag)
	} else if query != "" {
		scripts = scriptManager.SearchScripts(query)
	} else {
		scripts = scriptManager.ListEnabledScripts()
	}

	response := make([]map[string]interface{}, 0, len(scripts))
	for _, script := range scripts {
		response = append(response, map[string]interface{}{
			"name":        script.Metadata.Name,
			"version":     script.Metadata.Version,
			"description": script.Metadata.Description,
			"tags":        script.Metadata.Tags,
		})
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// InspectPythonProcess inspects a Python process
func InspectPythonProcess(c *gin.Context) {
	var req pythoninspector.InspectRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, err.Error(), nil))
		return
	}

	inspector := pythoninspector.GetInspector()
	if inspector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "inspector not initialized", nil))
		return
	}

	result, err := inspector.InspectWithScripts(
		c.Request.Context(),
		req.PID,
		req.Scripts,
		req.Timeout,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}

// ListPythonProcesses lists all Python processes
func ListPythonProcesses(c *gin.Context) {
	inspector := pythoninspector.GetInspector()
	if inspector == nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, "inspector not initialized", nil))
		return
	}

	processes, err := inspector.ListPythonProcesses()
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c, http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, processes))
}

// GetProcessStatus checks if a process exists
func GetProcessStatus(c *gin.Context) {
	pidStr := c.Param("pid")
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c, http.StatusBadRequest, "invalid PID", nil))
		return
	}

	// Check if process exists
	if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err != nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c, http.StatusNotFound, "process not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"pid":    pid,
		"exists": true,
	}))
}
