package handlers

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

var taskFacade database.WorkloadTaskFacadeInterface

// InitTaskMonitor 初始化任务监控
func InitTaskMonitor() {
	taskFacade = database.NewWorkloadTaskFacade()
	log.Info("Task monitor handler initialized")
}

// GetTaskStatistics 获取任务统计信息
// @Summary Get task statistics
// @Description Returns statistics about all tasks
// @Tags task-monitor
// @Produce json
// @Success 200 {object} rest.Response
// @Router /tasks/stats [get]
func GetTaskStatistics(c *gin.Context) {
	ctx := c.Request.Context()

	// 统计各种状态的任务数量
	stats := map[string]interface{}{
		"by_status": make(map[string]int),
		"by_type":   make(map[string]int),
	}

	// 查询所有任务类型
	taskTypes := []string{
		constant.TaskTypeDetection,
		constant.TaskTypeMetadataCollection,
		constant.TaskTypeTensorBoardStream,
	}

	totalTasks := 0
	for _, taskType := range taskTypes {
		// 查询各状态的任务
		for _, status := range []string{
			constant.TaskStatusPending,
			constant.TaskStatusRunning,
			constant.TaskStatusCompleted,
			constant.TaskStatusFailed,
		} {
			// 这里简化实现，实际可以用 SQL 聚合查询
			tasks, _ := taskFacade.ListTasksByStatus(ctx, status)
			count := 0
			for _, task := range tasks {
				if task.TaskType == taskType {
					count++
				}
			}
			
			if count > 0 {
				stats["by_status"].(map[string]int)[status] += count
				stats["by_type"].(map[string]int)[taskType] += count
				totalTasks += count
			}
		}
	}

	stats["total_tasks"] = totalTasks

	c.JSON(http.StatusOK, rest.SuccessResp(c, stats))
}

// ListAllTasks 列出所有任务
// @Summary List all tasks
// @Description Lists all tasks with optional filters
// @Tags task-monitor
// @Param status query string false "Filter by status"
// @Param task_type query string false "Filter by task type"
// @Produce json
// @Success 200 {object} rest.Response
// @Router /tasks [get]
func ListAllTasks(c *gin.Context) {
	ctx := c.Request.Context()

	status := c.Query("status")
	taskType := c.Query("task_type")

	var tasks []*model.WorkloadTaskState
	var err error

	if status != "" {
		tasks, err = taskFacade.ListTasksByStatus(ctx, status)
	} else {
		// 查询所有状态的任务
		allTasks := []*model.WorkloadTaskState{}
		for _, s := range []string{
			constant.TaskStatusPending,
			constant.TaskStatusRunning,
			constant.TaskStatusCompleted,
			constant.TaskStatusFailed,
		} {
			statusTasks, _ := taskFacade.ListTasksByStatus(ctx, s)
			allTasks = append(allTasks, statusTasks...)
		}
		tasks = allTasks
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			ctx,
			http.StatusInternalServerError,
			"failed to list tasks",
			err,
		))
		return
	}

	// 过滤 task_type
	if taskType != "" {
		filtered := []*model.WorkloadTaskState{}
		for _, task := range tasks {
			if task.TaskType == taskType {
				filtered = append(filtered, task)
			}
		}
		tasks = filtered
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"tasks": tasks,
		"total": len(tasks),
	}))
}

// GetTask 获取特定任务详情
// @Summary Get task details
// @Description Gets detailed information about a specific task
// @Tags task-monitor
// @Param workload_uid path string true "Workload UID"
// @Param task_type path string true "Task Type"
// @Produce json
// @Success 200 {object} rest.Response
// @Failure 404 {object} rest.Response
// @Router /tasks/{workload_uid}/{task_type} [get]
func GetTask(c *gin.Context) {
	ctx := c.Request.Context()
	workloadUID := c.Param("workload_uid")
	taskType := c.Param("task_type")

	if workloadUID == "" || taskType == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			ctx,
			http.StatusBadRequest,
			"workload_uid and task_type are required",
			nil,
		))
		return
	}

	task, err := taskFacade.GetTask(ctx, workloadUID, taskType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			ctx,
			http.StatusInternalServerError,
			"failed to get task",
			err,
		))
		return
	}

	if task == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(
			ctx,
			http.StatusNotFound,
			"task not found",
			nil,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, task))
}

// ListWorkloadTasks 列出某个 workload 的所有任务
// @Summary List workload tasks
// @Description Lists all tasks for a specific workload
// @Tags task-monitor
// @Param workload_uid path string true "Workload UID"
// @Produce json
// @Success 200 {object} rest.Response
// @Router /tasks/workload/{workload_uid} [get]
func ListWorkloadTasks(c *gin.Context) {
	ctx := c.Request.Context()
	workloadUID := c.Param("workload_uid")

	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(
			ctx,
			http.StatusBadRequest,
			"workload_uid is required",
			nil,
		))
		return
	}

	tasks, err := taskFacade.ListTasksByWorkload(ctx, workloadUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			ctx,
			http.StatusInternalServerError,
			"failed to list tasks",
			err,
		))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"workload_uid": workloadUID,
		"tasks":        tasks,
		"total":        len(tasks),
	}))
}

// GetActiveStreams 获取活跃的流式任务
// @Summary Get active streams
// @Description Gets all active TensorBoard streaming tasks
// @Tags task-monitor
// @Produce json
// @Success 200 {object} rest.Response
// @Router /tasks/streams/active [get]
func GetActiveStreams(c *gin.Context) {
	ctx := c.Request.Context()

	// 查询运行中的 TensorBoard 流式任务
	tasks, err := taskFacade.ListTasksByStatus(ctx, constant.TaskStatusRunning)
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(
			ctx,
			http.StatusInternalServerError,
			"failed to list tasks",
			err,
		))
		return
	}

	// 过滤出 TensorBoard 流式任务
	streamTasks := []*model.WorkloadTaskState{}
	for _, task := range tasks {
		if task.TaskType == constant.TaskTypeTensorBoardStream {
			streamTasks = append(streamTasks, task)
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"streams": streamTasks,
		"total":   len(streamTasks),
	}))
}

