package informer

import (
	"context"
	"errors"
	"fmt"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/informers/externalversions"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NewWorkloadInformer creates a new workload informer instance.
func NewWorkloadInformer(c client.Client) *WorkloadInformer {
	return &WorkloadInformer{
		Client: c,
	}
}

type WorkloadInformer struct {
	client.Client
}

// OnAdd is called when a workload is added.
func (w *WorkloadInformer) OnAdd(obj interface{}, isInInitialList bool) {
	workload, ok := obj.(*v1.Workload)
	if !ok {
		klog.Errorf("Failed to convert obj to Workload")
		return
	}
	if isInInitialList {
		return
	}
	ctx := context.Background()
	if len(workload.Status.Conditions) == 0 {
		return
	}
	newestCondition := workload.Status.Conditions[len(workload.Status.Conditions)-1]
	uid := fmt.Sprintf("%s-%s-%s", workload.Name, newestCondition.Type, newestCondition.Reason)

	notifyData := map[string]interface{}{
		"topic":     model.TopicWorkload,
		"condition": newestCondition.Type,
		"workload":  workload,
	}
	userId := v1.GetUserId(workload)
	user := &v1.User{}
	err := w.Get(ctx, client.ObjectKey{Name: userId}, user)
	if err != nil {
		klog.Errorf("Failed to get user %s: %v", userId, err)
		return
	}
	notifyData["users"] = []*v1.User{user}

	notificationManager := notification.GetNotificationManager()
	err = notificationManager.SubmitNotification(ctx, model.TopicWorkload, uid, notifyData)
	if err != nil {
		klog.Errorf("Failed to submit notification for workload %s: %v", workload.Name, err)
		return
	}
}

// OnUpdate is called when a workload is updated.
func (w *WorkloadInformer) OnUpdate(oldObj, newObj interface{}) {
	workload, ok := newObj.(*v1.Workload)
	if !ok {
		klog.Errorf("Failed to convert obj to Workload")
		return
	}
	oldWorkload, ok := oldObj.(*v1.Workload)
	if !ok {
		klog.Errorf("Failed to convert oldObj to Workload")
		return
	}
	ctx := context.Background()
	if len(workload.Status.Conditions) == 0 {
		return
	}
	if len(oldWorkload.Status.Conditions) == len(workload.Status.Conditions) {
		return
	}
	newestCondition := workload.Status.Conditions[len(workload.Status.Conditions)-1]
	uid := fmt.Sprintf("%s-%s-%s", workload.Name, newestCondition.Type, newestCondition.Reason)

	notifyData := map[string]interface{}{
		"topic":     model.TopicWorkload,
		"condition": newestCondition.Type,
		"workload":  workload,
	}
	userId := v1.GetUserId(workload)
	user := &v1.User{}
	err := w.Get(ctx, client.ObjectKey{Name: userId}, user)
	if err != nil {
		klog.Errorf("Failed to get user %s: %v", userId, err)
		return
	}
	notifyData["users"] = []*v1.User{user}

	// Submit notification
	notificationManager := notification.GetNotificationManager()
	err = notificationManager.SubmitNotification(ctx, model.TopicWorkload, uid, notifyData)
	if err != nil {
		klog.Errorf("Failed to submit notification for workload %s: %v", workload.Name, err)
		return
	}
}

// OnDelete is called when a workload is deleted.
func (w *WorkloadInformer) OnDelete(obj interface{}) {
	return
}

// Register registers the informer with the factory.
func (w *WorkloadInformer) Register(factory externalversions.SharedInformerFactory) error {
	_, err := factory.Amd().V1().Workloads().Informer().AddEventHandler(w)
	if err != nil {
		klog.Errorf("Failed to register WorkloadInformer")
		return errors.New(fmt.Sprintf("Failed to register WorkloadInformer for informer: %s", err))
	}
	return nil
}
