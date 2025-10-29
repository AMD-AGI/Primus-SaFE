package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/database/dal"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/filter"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/sql"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
)

func getDB() *gorm.DB {
	return sql.GetDefaultDB()
}

func CreateNode(ctx context.Context, node *model.Node) error {
	return dal.Use(getDB()).Node.WithContext(ctx).Create(node)
}

func UpdateNode(ctx context.Context, node *model.Node) error {
	return dal.Use(getDB()).Node.WithContext(ctx).Save(node)
}

func GetNodeByName(ctx context.Context, name string) (*model.Node, error) {
	query := dal.Use(getDB()).Node
	node, err := query.WithContext(ctx).Where(query.Name.Eq(name)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return node, nil
}

func SearchNode(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
	db := getDB()
	q := dal.Use(db).Node
	query := q.WithContext(ctx)

	if f.Name != nil {
		query = query.Where(q.Name.Like(fmt.Sprintf("%%%s%%", *f.Name)))
	}
	if f.Address != nil {
		query = query.Where(q.Address.Eq(*f.Address))
	}
	if f.GPUName != nil {
		query = query.Where(q.GpuName.Like(fmt.Sprintf("%%%s%%", *f.GPUName)))
	}
	if f.GPUAllocation != nil {
		query = query.Where(q.GpuAllocation.Eq(int32(*f.GPUAllocation)))
	}
	if f.GPUCount != nil {
		query = query.Where(q.GpuCount.Eq(int32(*f.GPUCount)))
	}
	if f.GPUUtilMin != nil {
		query = query.Where(q.GpuUtilization.Gte(*f.GPUUtilMin))
	}
	if f.GPUUtilMax != nil {
		query = query.Where(q.GpuUtilization.Lte(*f.GPUUtilMax))
	}
	if len(f.Status) > 0 {
		query = query.Where(q.Status.In(f.Status...))
	}
	if f.CPU != nil {
		query = query.Where(q.CPU.Eq(*f.CPU))
	}
	if f.CPUCount != nil {
		query = query.Where(q.CPUCount.Eq(int32(*f.CPUCount)))
	}
	if f.Memory != nil {
		query = query.Where(q.Memory.Eq(*f.Memory))
	}
	if f.K8sVersion != nil {
		query = query.Where(q.K8sVersion.Eq(*f.K8sVersion))
	}
	if f.K8sStatus != nil {
		query = query.Where(q.K8sStatus.Eq(*f.K8sStatus))
	}
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	gormDB := query.UnderlyingDB()
	if f.OrderBy != "" {
		order := f.Order
		if order == "" {
			order = "DESC"
		}
		gormDB = gormDB.Order(fmt.Sprintf("%s %s", f.OrderBy, order))
	}

	if f.Limit > 0 {
		gormDB = gormDB.Limit(f.Limit)
	}
	if f.Offset > 0 {
		gormDB = gormDB.Offset(f.Offset)
	}
	var nodes []*model.Node
	err = gormDB.Find(&nodes).Error
	if err != nil {
		return nil, 0, err
	}
	return nodes, int(count), nil
}

func GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*model.GpuDevice, error) {
	q := dal.Use(getDB()).GpuDevice
	device, err := q.WithContext(ctx).Where(q.NodeID.Eq(nodeId)).Where(q.GpuID.Eq(int32(gpuId))).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return device, nil

}

func CreateGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return dal.Use(getDB()).GpuDevice.WithContext(ctx).Create(device)
}

func UpdateGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return dal.Use(getDB()).GpuDevice.WithContext(ctx).Save(device)
}

func ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.GpuDevice, error) {
	q := dal.Use(getDB()).GpuDevice
	return q.WithContext(ctx).Where(q.NodeID.Eq(nodeId)).Order(q.GpuID.Asc()).Find()
}

func CreateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error {
	return dal.Use(getDB()).GpuPods.WithContext(ctx).Create(gpuPods)
}

func UpdateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error {
	return dal.Use(getDB()).GpuPods.WithContext(ctx).Save(gpuPods)
}

func CreateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPodsEvent) error {
	return dal.Use(getDB()).GpuPodsEvent.WithContext(ctx).Create(gpuPods)
}

func UpdateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPods) error {
	return dal.Use(getDB()).GpuPods.WithContext(ctx).Save(gpuPods)
}

func GetGpuPodsByPodUid(ctx context.Context, podUid string) (*model.GpuPods, error) {
	q := dal.Use(getDB()).GpuPods
	result, err := q.WithContext(ctx).Where(q.UID.Eq(podUid)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error {
	return dal.Use(getDB()).PodSnapshot.WithContext(ctx).Create(podSnapshot)
}

func UpdatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error {
	return dal.Use(getDB()).PodSnapshot.WithContext(ctx).Save(podSnapshot)
}

func GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*model.PodSnapshot, error) {
	q := dal.Use(getDB()).PodSnapshot
	result, err := dal.Use(getDB()).
		PodSnapshot.
		WithContext(ctx).
		Where(q.PodUID.Eq(podUid)).
		Where(q.ResourceVersion.Lt(int32(resourceVersion))).
		Order(q.ResourceVersion.Desc()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error {
	return dal.Use(getDB()).GpuWorkloadSnapshot.WithContext(ctx).Create(gpuWorkloadSnapshot)
}

func UpdateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error {
	return dal.Use(getDB()).GpuWorkloadSnapshot.WithContext(ctx).Save(gpuWorkloadSnapshot)
}

func GetLatestGpuWorkloadSnapshotByUid(ctx context.Context, uid string, resourceVersion int) (*model.GpuWorkloadSnapshot, error) {
	q := dal.Use(getDB()).GpuWorkloadSnapshot
	result, err := q.
		WithContext(ctx).
		Where(q.UID.Eq(uid)).
		Where(q.ResourceVersion.Lt(int32(resourceVersion))).
		Order(q.ResourceVersion.Desc()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func GetGpuWorkloadByUid(ctx context.Context, uid string) (*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	result, err := q.WithContext(ctx).Where(q.UID.Eq(uid)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	return dal.Use(getDB()).GpuWorkload.WithContext(ctx).Create(gpuWorkload)
}

func UpdateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	return dal.Use(getDB()).GpuWorkload.WithContext(ctx).Save(gpuWorkload)
}

func CreateWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error {
	ref := &model.WorkloadPodReference{
		WorkloadUID: workloadUid,
		PodUID:      podUid,
		CreatedAt:   time.Now(),
	}
	return dal.Use(getDB()).WorkloadPodReference.WithContext(ctx).Create(ref)
}

func GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*model.GpuPods, error) {
	q := dal.Use(getDB()).GpuPods
	result, err := q.WithContext(ctx).Where(q.NodeName.Eq(nodeName)).Where(q.Running.Is(true)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	return result, nil
}

func GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*model.GpuPods, int, error) {
	q := dal.Use(getDB()).GpuPods
	query := q.
		WithContext(ctx).
		Where(q.NodeName.Eq(nodeName)).
		Where(q.Running.Is(false)).
		Where(q.Phase.Neq(string(corev1.PodPending)))
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	result, err := query.Order(q.CreatedAt.Desc()).Offset((pageNum - 1) * pageSize).Limit(pageSize).Find()
	if err != nil {
		return nil, 0, err
	}
	return result, int(count), nil
}

func ListWorkloadPodReferencesByPodUids(ctx context.Context, podUids []string) ([]*model.WorkloadPodReference, error) {
	q := dal.Use(getDB()).WorkloadPodReference
	refs, err := q.WithContext(ctx).Where(q.PodUID.In(podUids...)).Find()
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func ListTopLevelWorkloadByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	workloads, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Where(q.ParentUID.Eq("")).Find()
	if err != nil {
		return nil, err
	}
	return workloads, nil
}

func ListWorkloadPodReferenceByWorkloadUid(ctx context.Context, workloadUid string) ([]*model.WorkloadPodReference, error) {
	q := dal.Use(getDB()).WorkloadPodReference
	refs, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Find()
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func ListActivePodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) {
	q := dal.Use(getDB()).GpuPods
	pods, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Where(q.Running.Is(true)).Find()
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func ListPodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) {
	q := dal.Use(getDB()).GpuPods
	pods, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Find()
	if err != nil {
		return nil, err
	}
	return pods, nil
}

func QueryWorkload(ctx context.Context, f *filter.WorkloadFilter) ([]*model.GpuWorkload, int, error) {
	q := dal.Use(getDB()).GpuWorkload
	query := q.WithContext(ctx)
	if f.Kind != nil {
		query = query.Where(q.Kind.Eq(*f.Kind))
	}
	if f.Namespace != nil {
		query = query.Where(q.Namespace.Eq(*f.Namespace))
	}
	if f.Name != nil {
		query = query.Where(q.Name.Like(fmt.Sprintf("%%%s%%", *f.Name)))
	}
	if f.Uid != nil {
		query = query.Where(q.UID.Eq(*f.Uid))
	}
	if f.ParentUid != nil {
		query = query.Where(q.ParentUID.Eq(*f.ParentUid))
	} else {
		query = query.Where(q.ParentUID.Eq(""))
	}
	if f.Status != nil {
		query = query.Where(q.Status.Eq(*f.Status))
	}
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	gormDB := query.UnderlyingDB()
	if f.OrderBy != "" {
		order := f.Order
		if order == "" {
			order = "DESC"
		}
		gormDB = gormDB.Order(fmt.Sprintf("%s %s", f.OrderBy, order))
	} else {
		gormDB = gormDB.Order("created_at desc")
	}

	if f.Limit > 0 {
		gormDB = gormDB.Limit(f.Limit)
	}
	if f.Offset > 0 {
		gormDB = gormDB.Offset(f.Offset)
	}
	var workloads []*model.GpuWorkload
	err = gormDB.Find(&workloads).Error
	if err != nil {
		return nil, 0, err
	}
	return workloads, int(count), nil
}

func GetWorkloadsNamespaceList(ctx context.Context) ([]string, error) {
	q := dal.Use(getDB()).GpuWorkload
	var namespaces []string
	err := q.WithContext(ctx).
		Distinct(q.Namespace).
		Pluck(q.Namespace, &namespaces)
	if err != nil {
		return nil, err
	}

	return namespaces, nil
}

func GetWorkloadKindList(ctx context.Context) ([]string, error) {
	q := dal.Use(getDB()).GpuWorkload
	var kinds []string
	err := q.WithContext(ctx).
		Distinct(q.Kind).
		Where(q.ParentUID.Eq("")).
		Pluck(q.Kind, &kinds)
	if err != nil {
		return nil, err
	}
	return kinds, nil
}

func GetWorkloadNotEnd(ctx context.Context) ([]*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	result, err := q.WithContext(ctx).Where(q.EndAt.IsNull()).Or(q.EndAt.Eq(time.Time{})).Find()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func GetPodResourceByUid(ctx context.Context, uid string) (*model.PodResource, error) {
	q := dal.Use(getDB()).PodResource
	result, err := q.WithContext(ctx).Where(q.UID.Eq(uid)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return dal.Use(getDB()).PodResource.WithContext(ctx).Create(podResource)
}

func UpdatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return dal.Use(getDB()).PodResource.WithContext(ctx).Save(podResource)
}

func CreateNodeContainer(ctx context.Context, nodeContainer *model.NodeContainer) error {
	return dal.Use(getDB()).NodeContainer.WithContext(ctx).Create(nodeContainer)
}

func UpdateNodeContainer(ctx context.Context, nodeContainer *model.NodeContainer) error {
	return dal.Use(getDB()).NodeContainer.WithContext(ctx).Save(nodeContainer)
}

func GetNodeContainerByContainerId(ctx context.Context, containerId string) (*model.NodeContainer, error) {
	q := dal.Use(getDB()).NodeContainer
	result, err := q.WithContext(ctx).Where(q.ContainerID.Eq(containerId)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateNodeContainerDevice(ctx context.Context, nodeContainerDevice *model.NodeContainerDevices) error {
	return dal.Use(getDB()).NodeContainerDevices.WithContext(ctx).Create(nodeContainerDevice)
}

func UpdateNodeContainerDevice(ctx context.Context, nodeContainerDevice *model.NodeContainerDevices) error {
	return dal.Use(getDB()).NodeContainerDevices.WithContext(ctx).Save(nodeContainerDevice)
}

func GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx context.Context, containerId, deviceUid string) (*model.NodeContainerDevices, error) {
	q := dal.Use(getDB()).NodeContainerDevices
	result, err := q.WithContext(ctx).
		Where(q.ContainerID.Eq(containerId)).
		Where(q.DeviceUUID.Eq(deviceUid)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateNodeContainerEvent(ctx context.Context, nodeContainerEvent *model.NodeContainerEvent) error {
	return dal.Use(getDB()).NodeContainerEvent.WithContext(ctx).Create(nodeContainerEvent)
}

func ListActiveGpuPods(ctx context.Context) ([]*model.GpuPods, error) {
	q := dal.Use(getDB()).GpuPods
	result, err := q.WithContext(ctx).Where(q.Running.Is(true)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func ListRunningContainersByPodUid(ctx context.Context, podUid string) ([]*model.NodeContainer, error) {
	q := dal.Use(getDB()).NodeContainer
	containers, err := q.WithContext(ctx).
		Where(q.PodUID.Eq(podUid)).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return containers, nil
}

func ListContainerDevicesByContainerId(ctx context.Context, containerId string) ([]*model.NodeContainerDevices, error) {
	q := dal.Use(getDB()).NodeContainerDevices
	devices, err := q.WithContext(ctx).Where(q.ContainerID.Eq(containerId)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return devices, nil
}

func ListRunningWorkload(ctx context.Context) ([]*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	workloads, err := q.WithContext(ctx).Where(q.Status.In(metadata.WorkloadStatusRunning)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return workloads, nil
}

func ListWorkloadsByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	results, err := q.WithContext(ctx).Where(q.UID.In(uids...)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

func GetNearestWorkloadByPodUid(ctx context.Context, podUid string) (*model.GpuWorkload, error) {
	workloadRefs, err := ListWorkloadPodReferencesByPodUids(ctx, []string{podUid})
	if err != nil {
		return nil, err
	}
	if len(workloadRefs) == 0 {
		return nil, nil
	}
	workloadUids := []string{}
	for _, ref := range workloadRefs {
		workloadUids = append(workloadUids, ref.WorkloadUID)
	}
	workloads, err := ListWorkloadsByUids(ctx, workloadUids)
	if err != nil {
		return nil, err
	}
	leaves := findLeafWorkloads(workloads)
	if len(leaves) == 0 {
		return nil, nil
	}
	return leaves[0], nil
}

func findLeafWorkloads(workloads []*model.GpuWorkload) []*model.GpuWorkload {
	parentSet := make(map[string]bool)
	for _, w := range workloads {
		if w.ParentUID != "" {
			parentSet[w.ParentUID] = true
		}
	}

	var leaves []*model.GpuWorkload
	for _, w := range workloads {
		if !parentSet[w.UID] {
			leaves = append(leaves, w)
		}
	}
	return leaves
}

func GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx context.Context, workloadUid, nearestWorkloadId, typ string) (*model.WorkloadEvent, error) {
	q := dal.Use(getDB()).WorkloadEvent
	result, err := q.
		WithContext(ctx).
		Where(q.WorkloadUID.Eq(workloadUid)).
		Where(q.NearestWorkloadUID.Eq(nearestWorkloadId)).
		Where(q.Type.Eq(typ)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil

}

func CreateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error {
	return dal.Use(getDB()).WorkloadEvent.WithContext(ctx).Create(workloadEvent)
}

func UpdateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error {
	return dal.Use(getDB()).WorkloadEvent.WithContext(ctx).Create(workloadEvent)
}

func GetLatestEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error) {
	q := dal.Use(getDB()).WorkloadEvent
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Where(q.NearestWorkloadUID.Eq(nearestWorkloadId)).Order(q.CreatedAt.Desc()).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func GetLatestOtherWorkloadEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error) {
	q := dal.Use(getDB()).WorkloadEvent
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Where(q.NearestWorkloadUID.Neq(nearestWorkloadId)).Order(q.CreatedAt.Desc()).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx context.Context, workloadUid string, serial int, iteration int) (*model.TrainingPerformance, error) {
	q := dal.Use(getDB()).TrainingPerformance
	result, err := q.WithContext(ctx).Where(q.Serial.Eq(int32(serial))).Where(q.Iteration.Eq(int32(iteration))).Where(q.WorkloadUID.Eq(workloadUid)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateTrainingPerformance(ctx context.Context, trainingPerformance *model.TrainingPerformance) error {
	return dal.Use(getDB()).TrainingPerformance.WithContext(ctx).Create(trainingPerformance)
}

func ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx context.Context, workloadUid string, start, end time.Time) ([]*model.TrainingPerformance, error) {
	q := dal.Use(getDB()).TrainingPerformance
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Where(q.CreatedAt.Gte(start)).Where(q.CreatedAt.Lte(end)).Order(q.CreatedAt.Asc()).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*model.RdmaDevice, error) {
	q := dal.Use(getDB()).RdmaDevice
	result, err := q.WithContext(ctx).Where(q.NodeGUID.Eq(nodeGuid)).Where(q.IfIndex.Eq(int32(port))).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateRdmaDevice(ctx context.Context, rdmaDevice *model.RdmaDevice) error {
	return dal.Use(getDB()).RdmaDevice.WithContext(ctx).Create(rdmaDevice)
}

func ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.RdmaDevice, error) {
	q := dal.Use(getDB()).RdmaDevice
	results, err := q.WithContext(ctx).Where(q.NodeID.Eq(nodeId)).Order(q.IfIndex.Asc()).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

func DeleteRdmaDeviceById(ctx context.Context, id int32) error {
	q := dal.Use(getDB()).RdmaDevice
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

func CreateNodeDeviceChangelog(ctx context.Context, changelog *model.NodeDeviceChangelog) error {
	return dal.Use(getDB()).NodeDeviceChangelog.WithContext(ctx).Create(changelog)
}

func DeleteGpuDeviceById(ctx context.Context, id int32) error {
	q := dal.Use(getDB()).GpuDevice
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

func GetStorageByKindAndName(ctx context.Context, kind, name string) (*model.Storage, error) {
	q := dal.Use(getDB()).Storage
	result, err := q.WithContext(ctx).Where(q.Kind.Eq(kind)).Where(q.Name.Eq(name)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func CreateStorage(ctx context.Context, storage *model.Storage) error {
	return dal.Use(getDB()).Storage.WithContext(ctx).Create(storage)
}

func UpdateStorage(ctx context.Context, storage *model.Storage) error {
	return dal.Use(getDB()).Storage.WithContext(ctx).Save(storage)
}

func ListStorage(ctx context.Context, pageNum, pageSize int) ([]*model.Storage, int, error) {
	q := dal.Use(getDB()).Storage
	query := q.WithContext(ctx)
	count, err := query.Count()
	if err != nil {
		return nil, 0, err
	}
	gormDB := query.UnderlyingDB()
	gormDB = gormDB.Order("created_at desc")

	if pageSize > 0 {
		gormDB = gormDB.Limit(pageSize)
	}
	if pageNum > 0 {
		gormDB = gormDB.Offset((pageNum - 1) * pageSize)
	}
	var storages []*model.Storage
	err = gormDB.Find(&storages).Error
	if err != nil {
		return nil, 0, err
	}
	return storages, int(count), nil
}

func ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx context.Context, workloadUids []string, start, end time.Time) ([]*model.TrainingPerformance, error) {
	q := dal.Use(getDB()).TrainingPerformance
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.In(workloadUids...)).Where(q.CreatedAt.Gte(start)).Where(q.CreatedAt.Lte(end)).Order(q.CreatedAt.Asc()).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func ListChildrenWorkloadByParentUid(ctx context.Context, parentUid string) ([]*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	results, err := q.WithContext(ctx).Where(q.ParentUID.Eq(parentUid)).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}

func ListWorkloadByLabelValue(ctx context.Context, labelKey, labelValue string) ([]*model.GpuWorkload, error) {
	result := []*model.GpuWorkload{}
	err := getDB().Raw(fmt.Sprintf(`SELECT * FROM gpu_workload WHERE label @> '{"%s": "%s"}'`), labelKey, labelValue).Scan(&result).Error
	if err != nil {
		return nil, err
	}

	return result, nil
}

func ListWorkloadNotEndByKind(ctx context.Context, kind string) ([]*model.GpuWorkload, error) {
	q := dal.Use(getDB()).GpuWorkload
	results, err := q.WithContext(ctx).
		Where(q.Kind.Eq(kind)).
		Where(q.EndAt.IsNull()).
		Or(q.EndAt.Eq(time.Time{})).
		Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}
