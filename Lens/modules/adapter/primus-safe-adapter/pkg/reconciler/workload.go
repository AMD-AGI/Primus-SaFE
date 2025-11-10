package reconciler

import (
	"context"
	"strconv"
	"time"

	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/constant"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	primusSafeConstant "github.com/AMD-AGI/primus-lens/primus-safe-adapter/pkg/constant"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type WorkloadReconciler struct {
	client *clientsets.K8SClientSet
}

func (r *WorkloadReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&primusSafeV1.Workload{}).
		Complete(r)
}

func (r *WorkloadReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	workload := &primusSafeV1.Workload{}
	err := r.client.ControllerRuntimeClient.Get(ctx, req.NamespacedName, workload)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	err = r.saveWorkloadToDB(ctx, workload)
	if err != nil {
		return reconcile.Result{}, err
	}
	if workload.DeletionTimestamp != nil {
		controllerutil.RemoveFinalizer(workload, constant.PrimusLensGpuWorkloadExporterFinalizer)
		err = r.client.ControllerRuntimeClient.Update(ctx, workload)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}
	if !controllerutil.ContainsFinalizer(workload, constant.PrimusLensGpuWorkloadExporterFinalizer) {
		controllerutil.AddFinalizer(workload, constant.PrimusLensGpuWorkloadExporterFinalizer)
		err = r.client.ControllerRuntimeClient.Update(ctx, workload)
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *WorkloadReconciler) calculateGpuRequest(ctx context.Context, workload *primusSafeV1.Workload) int32 {
	gpuPerReplica := 0
	gpuPerReplica, err := strconv.Atoi(workload.Spec.Resource.GPU)
	if err != nil {
		return 0
	}
	return int32(gpuPerReplica * workload.Spec.Resource.Replica)
}

func (r *WorkloadReconciler) saveWorkloadToDB(ctx context.Context, workload *primusSafeV1.Workload) error {
	existWorkload, err := database.GetFacade().GetWorkload().GetGpuWorkloadByUid(ctx, string(workload.UID))
	if err != nil {
		return err
	}
	dbWorkload := &model.GpuWorkload{
		GroupVersion: workload.GroupVersionKind().GroupVersion().String(),
		Kind:         workload.Kind,
		Namespace:    workload.Spec.Workspace,
		Name:         workload.Name,
		UID:          string(workload.UID),
		GpuRequest:   r.calculateGpuRequest(ctx, workload),
		CreatedAt:    workload.CreationTimestamp.Time,
		UpdatedAt:    time.Now(),
		Labels:       map[string]interface{}{},
		Source:       constant.ContainerSourceK8S,
		Status:       metadata.WorkloadStatusRunning,
	}
	for key, value := range workload.Labels {
		if primusSafeConstant.WorkloadDispatchCountLabel == key {
			count, _ := strconv.Atoi(value)
			dbWorkload.Labels[key] = count
		} else {
			dbWorkload.Labels[key] = value
		}
	}
	if workload.DeletionTimestamp != nil {
		dbWorkload.Status = metadata.WorkloadStatusDone
		dbWorkload.EndAt = workload.DeletionTimestamp.Time
	}
	if existWorkload == nil {
		return database.GetFacade().GetWorkload().CreateGpuWorkload(ctx, dbWorkload)
	}
	dbWorkload.ID = existWorkload.ID
	return database.GetFacade().GetWorkload().UpdateGpuWorkload(ctx, dbWorkload)
}
