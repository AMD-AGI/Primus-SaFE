package resource

import (
	"bytes"
	"context"
	"fmt"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func SetupImageImportJobReconciler(mgr ctrlruntime.Manager) error {
	if !commonconfig.IsDBEnable() {
		return nil
	}
	dbClient := dbclient.NewClient()
	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		klog.Errorf("Create kubernetes clientset failed: %v", err)
		return err
	}
	r := &ImageImportJobReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		dbClient:  dbClient,
		k8sClient: *clientSet,
	}
	err = ctrlruntime.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.TypedCreateEvent[client.Object]) bool {
				return filterImageImportJob(e.Object)
			},
			DeleteFunc: func(e event.TypedDeleteEvent[client.Object]) bool {
				return filterImageImportJob(e.Object)
			},
			UpdateFunc: func(e event.TypedUpdateEvent[client.Object]) bool {
				return filterImageImportJob(e.ObjectNew)
			},
		}).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup ImageImportJob Controller successfully")
	return nil
}

func filterImageImportJob(o client.Object) bool {
	if o.GetLabels() == nil {
		return false
	}
	_, exist := o.GetLabels()["primus-safe/image-import"]
	return exist
}

type ImageImportJobReconciler struct {
	*ClusterBaseReconciler
	dbClient  dbclient.Interface
	k8sClient kubernetes.Clientset
}

func (r *ImageImportJobReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	job := &batchv1.Job{}
	err := r.Client.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrlruntime.Result{}, nil
		}
		klog.Errorf("Get ImageImportJob %s/%s failed: %v", req.Namespace, req.Name, err)
		return ctrlruntime.Result{}, err
	}
	existImportingJob, err := r.dbClient.GetImageImportJobByJobName(ctx, job.Name)
	if err != nil {
		klog.Errorf("Get ImageImportJob %s from db failed: %v", job.Name, err)
		return ctrlruntime.Result{}, err
	}
	if existImportingJob == nil {
		klog.Warningf("Get No ImageImportJob %s from db.This is an unexpected case!", job.Name)
		return ctrlruntime.Result{}, nil
	}
	existImage, err := r.dbClient.GetImageByTag(ctx, existImportingJob.DstName)
	if err != nil {
		klog.Errorf("Get Image %s from db failed: %v", existImportingJob.DstName, err)
		return ctrlruntime.Result{}, err
	}
	if existImage == nil {
		klog.Warningf("Get Image %s from db failed", existImportingJob.DstName)
		return ctrlruntime.Result{}, nil
	}
	status := common.ImageImportingStatus
	if job.Status.Active == 0 && job.Status.Succeeded == 0 && job.Status.Failed == 0 {
		status = common.ImageImportingStatus
	} else if job.Status.Active == 1 {
		status = common.ImageImportingStatus
	} else if job.Status.Succeeded == 1 {
		status = common.ImageImportReadyStatus
	} else if job.Status.Failed >= 1 {
		status = common.ImageImportFailedStatus
	}
	existImage.Status = status
	err = r.dbClient.UpsertImage(ctx, existImage)
	if err != nil {
		klog.Errorf("Update Image %s status to %s failed: %v", existImage.Tag, status, err)
		return ctrlruntime.Result{}, err
	}
	if status == common.ImageImportFailedStatus {
		logs, err := r.getImportImagePodLogs(ctx, job)
		if err != nil {
			klog.Errorf("Get import image job %s pod logs failed: %v", job.Name, err)
			return ctrlruntime.Result{}, err
		}
		existImportingJob.Log = logs
	}
	err = r.dbClient.UpsertImageImportJob(ctx, existImportingJob)
	if err != nil {
		klog.Errorf("Update ImageImportJob %s status to %s failed: %v", existImportingJob.JobName, status, err)
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *ImageImportJobReconciler) getImportImagePodLogs(ctx context.Context, job *batchv1.Job) (string, error) {
	labelSelect, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		klog.Errorf("transform labelSelect as Selector failed: %v", err)
		return "", err
	}
	pods := &corev1.PodList{}
	err = r.Client.List(ctx, pods, &client.ListOptions{
		LabelSelector: labelSelect,
		Namespace:     job.Namespace,
	})
	if err != nil {
		klog.Errorf("get pod list failed, labelSelector is: %s, err is: %v", labelSelect.String(), err)
		return "", err
	}
	if len(pods.Items) != 1 {
		return "", fmt.Errorf("expect 1 pod, but got %d", len(pods.Items))
	}

	pod := pods.Items[0]

	// 获取 pod 日志
	resp, err := r.k8sClient.CoreV1().Pods(job.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp); err != nil {
		return "", err
	}
	return buf.String(), nil
}
