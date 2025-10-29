package listener

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/constant"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Listener struct {
	apiVersion string
	kind       string
	namespace  string
	name       string
	uid        string

	client *clientsets.K8SClientSet
	gvr    schema.GroupVersionResource

	end bool
}

func NewListener(
	apiVersion, kind, namespace, name, uid string,
	client *clientsets.K8SClientSet,
) (*Listener, error) {
	gvr, err := k8sUtil.GvkToGvr(apiVersion, kind, client.Clientsets)
	if err != nil {
		return nil, err
	}

	return &Listener{
		apiVersion: apiVersion,
		kind:       kind,
		namespace:  namespace,
		name:       name,
		uid:        uid,
		client:     client,
		gvr:        gvr,
	}, nil
}

func (l *Listener) Start(ctx context.Context) {
	log.Infof("Starting listener for %s/%s (%s)", l.namespace, l.name, l.uid)
	go func() {
		defer func() {
			log.Infof("Listener for %s/%s (%s) stopped", l.namespace, l.name, l.uid)
			l.end = true
		}()
		err := l.Run(ctx)
		if err != nil {
			log.Errorf("Listener for %s/%s (%s) exited with error: %v", l.namespace, l.name, l.uid, err)
		}
	}()
}

func (l *Listener) IsEnd() bool {
	return l.end
}

func (l *Listener) Run(ctx context.Context) error {
	log.Infof("Listener for %s/%s (%s)", l.namespace, l.name, l.uid)
	watcher, err := l.client.Dynamic.Resource(l.gvr).
		Namespace(l.namespace).
		Watch(context.TODO(), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("metadata.name=%s", l.name),
		})
	if client.IgnoreNotFound(err) != nil {
		log.Errorf("Failed to start watcher for %s/%s: %v", l.namespace, l.name, err)
		return err
	} else if err != nil {
		err = l.setWorkloadEnd(ctx, l.uid)
		if err != nil {
			log.Errorf("Failed to set workload end for %s/%s (%s): %v", l.namespace, l.name, l.uid, err)
		}
		log.Infof("Workload %s/%s not found, exiting listener", l.namespace, l.name)
		return nil
	}
	defer watcher.Stop()
	log.Infof("Listener for %s/%s (%s) started", l.namespace, l.name, l.uid)
	ch := watcher.ResultChan()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return fmt.Errorf("watch channel closed for %s/%s", l.namespace, l.name)
			}
			obj := e.Object.(*unstructured.Unstructured)

			if l.uid != "" && string(obj.GetUID()) != l.uid {
				log.Warnf("Skip event for %s/%s: uid mismatch (expected %s, got %s)", l.namespace, l.name, l.uid, obj.GetUID())
				continue
			}
			log.Infof("Event for %s/%s (%s) Type %s", l.namespace, l.name, l.uid, e.Type)
			switch e.Type {
			case watch.Added, watch.Modified, watch.Deleted:
				// 确保有Finalizer
				if err := l.ensureFinalizer(ctx, obj); err != nil {
					log.Errorf("Failed to ensure finalizer for %s/%s: %v", l.namespace, l.name, err)
				}
				if obj.GetDeletionTimestamp() != nil {
					if err := l.saveGpuWorkload(ctx, obj); err != nil {
						log.Errorf("Failed to save workload %s/%s: %v", l.namespace, l.name, err)
					}
					if err := l.removeFinalizer(ctx, obj); err != nil {
						log.Errorf("Failed to remove finalizer for %s/%s: %v", l.namespace, l.name, err)
					}
					return nil
				} else {
					if err := l.saveGpuWorkload(ctx, obj); err != nil {
						log.Errorf("Failed to save workload %s/%s: %v", l.namespace, l.name, err)
					}
				}
			case watch.Bookmark:
				// ignore
			case watch.Error:
				return fmt.Errorf("watch error on %s/%s", l.namespace, l.name)
			}

		case <-ctx.Done():
			return fmt.Errorf("context done while watching %s/%s", l.namespace, l.name)
		case <-ticker.C:
			// 定期检查资源是否存在
			obj, err := l.client.Dynamic.Resource(l.gvr).
				Namespace(l.namespace).
				Get(ctx, l.name, metav1.GetOptions{})
			if client.IgnoreNotFound(err) != nil {
				log.Errorf("Failed to get %s/%s: %v", l.namespace, l.name, err)
			} else if err != nil {
				// 资源不存在，退出监听
				log.Infof("Resource %s/%s not found, exiting listener", l.namespace, l.name)
				if err := l.setWorkloadEnd(ctx, l.uid); err != nil {
					log.Errorf("Failed to set workload end for %s/%s (%s): %v", l.namespace, l.name, l.uid, err)
				}
				return nil
			} else if obj.GetDeletionTimestamp() != nil {
				if err := l.saveGpuWorkload(ctx, obj); err != nil {
					log.Errorf("Failed to save workload %s/%s: %v", l.namespace, l.name, err)
				}
				if err := l.removeFinalizer(ctx, obj); err != nil {
					log.Errorf("Failed to remove finalizer for %s/%s: %v", l.namespace, l.name, err)
				}
				return nil
			}
		}
	}
}

// 确保Finalizer存在
func (l *Listener) ensureFinalizer(ctx context.Context, obj *unstructured.Unstructured) error {
	finalizers := obj.GetFinalizers()
	for _, f := range finalizers {
		if f == constant.PrimusLensGpuWorkloadExporterFinalizer {
			return nil
		}
	}
	obj.SetFinalizers(append(finalizers, constant.PrimusLensGpuWorkloadExporterFinalizer))

	_, err := l.client.Dynamic.Resource(l.gvr).
		Namespace(l.namespace).
		Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

// 移除Finalizer
func (l *Listener) removeFinalizer(ctx context.Context, obj *unstructured.Unstructured) error {
	finalizers := obj.GetFinalizers()
	newFinalizers := []string{}
	for _, f := range finalizers {
		if f != constant.PrimusLensGpuWorkloadExporterFinalizer {
			newFinalizers = append(newFinalizers, f)
		}
	}
	obj.SetFinalizers(newFinalizers)

	_, err := l.client.Dynamic.Resource(l.gvr).
		Namespace(l.namespace).
		Update(ctx, obj, metav1.UpdateOptions{})
	return err
}

func (l *Listener) saveGpuWorkload(ctx context.Context, obj *unstructured.Unstructured) error {
	parentUid := ""
	if len(obj.GetOwnerReferences()) > 0 {
		parentUid = string(obj.GetOwnerReferences()[0].UID)

	}
	gpuWorkload := &model.GpuWorkload{
		GroupVersion: obj.GetAPIVersion(),
		Kind:         obj.GetKind(),
		Namespace:    obj.GetNamespace(),
		Name:         obj.GetName(),
		UID:          string(obj.GetUID()),
		ParentUID:    parentUid,
		GpuRequest:   0,
		Status:       metadata.WorkloadStatusRunning,
		CreatedAt:    obj.GetCreationTimestamp().Time,
		UpdatedAt:    time.Now(),
	}
	if obj.GetDeletionTimestamp() != nil {
		gpuWorkload.EndAt = obj.GetDeletionTimestamp().Time
	}
	existGpuWorkload, err := database.GetGpuWorkloadByUid(ctx, string(obj.GetUID()))
	if err != nil {
		return err
	}
	if existGpuWorkload == nil {
		existGpuWorkload = gpuWorkload
	} else {
		gpuWorkload.ID = existGpuWorkload.ID
		gpuWorkload.ParentUID = existGpuWorkload.ParentUID
	}
	if existGpuWorkload.ID == 0 {
		err = database.CreateGpuWorkload(ctx, existGpuWorkload)

	} else {
		err = database.UpdateGpuWorkload(ctx, existGpuWorkload)
	}
	if err != nil {
		return err
	}
	return nil
}

func (l *Listener) setWorkloadEnd(ctx context.Context, uid string) error {
	existGpuWorkload, err := database.GetGpuWorkloadByUid(ctx, uid)
	if err != nil {
		return err
	}
	if existGpuWorkload == nil {
		return nil
	}
	existGpuWorkload.EndAt = existGpuWorkload.UpdatedAt
	return database.UpdateGpuWorkload(ctx, existGpuWorkload)
}
