package api

import (
	"fmt"
	"net/http"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

func getGpuDevice(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	name := ctx.Param("name")
	node, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, name)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Fail get node.", errors.CodeDatabaseError))
		return
	}
	if node == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
		return
	}
	devices, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().ListGpuDeviceByNodeId(ctx, node.ID)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Fail get devices", errors.CodeDatabaseError))
		return
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, batchCvtGpuDevice2GpuDeviceInfo(devices)))
}

func batchCvtGpuDevice2GpuDeviceInfo(dbModels []*dbModel.GpuDevice) []model.GpuDeviceInfo {
	var result []model.GpuDeviceInfo
	for _, dbGpuModel := range dbModels {
		result = append(result, cvtGpuDevice2GpuDeviceInfo(dbGpuModel))
	}
	return result
}

func cvtGpuDevice2GpuDeviceInfo(dbModel *dbModel.GpuDevice) model.GpuDeviceInfo {
	return model.GpuDeviceInfo{
		DeviceId:    int(dbModel.GpuID),
		Model:       dbModel.GpuModel,
		Memory:      fmt.Sprintf("%dGB", dbModel.Memory/1024),
		Utilization: dbModel.Utilization,
		Temperature: dbModel.Temperature,
		Power:       dbModel.Power,
	}
}
