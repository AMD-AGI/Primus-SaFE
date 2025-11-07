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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
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

	// Use the passed-in context instead of context.TODO() to ensure proper cancellation
	watcher, err := l.client.Dynamic.Resource(l.gvr).
		Namespace(l.namespace).
		Watch(ctx, metav1.ListOptions{
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
				// Object is being deleted, handle finalizer removal logic
				if obj.GetDeletionTimestamp() != nil {
					log.Infof("Object %s/%s is being deleted, saving workload and removing finalizer", l.namespace, l.name)

					// Save the final workload state
					if err := l.saveGpuWorkload(ctx, obj); err != nil {
						log.Errorf("Failed to save workload %s/%s: %v", l.namespace, l.name, err)
					}

					// Try to remove finalizer, retry if failed
					if err := l.removeFinalizer(ctx, obj); err != nil {
						log.Errorf("Failed to remove finalizer for %s/%s: %v", l.namespace, l.name, err)
						// Don't return immediately, let periodic check retry
						continue
					}

					// Successfully removed finalizer, exit listener
					log.Infof("Successfully removed finalizer for %s/%s, exiting listener", l.namespace, l.name)
					return nil
				} else {
					/*
						// Object is running normally, ensure finalizer exists
						if err := l.ensureFinalizer(ctx, obj); err != nil {
							log.Errorf("Failed to ensure finalizer for %s/%s: %v", l.namespace, l.name, err)
							// Don't exit if it's a retriable error
							if !isRetriableError(err) {
								log.Warnf("Non-retriable error ensuring finalizer for %s/%s, continuing anyway: %v", l.namespace, l.name, err)
							}
						}
					*/
					// Save workload state
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
			// Periodically check if resource exists
			obj, err := l.client.Dynamic.Resource(l.gvr).
				Namespace(l.namespace).
				Get(ctx, l.name, metav1.GetOptions{})

			if apierrors.IsNotFound(err) {
				// Resource not found, mark workload as ended and exit listener
				log.Infof("Resource %s/%s not found during periodic check, exiting listener", l.namespace, l.name)
				if err := l.setWorkloadEnd(ctx, l.uid); err != nil {
					log.Errorf("Failed to set workload end for %s/%s (%s): %v", l.namespace, l.name, l.uid, err)
				}
				return nil
			}

			if err != nil {
				// Other errors (e.g., network errors), log but continue listening
				if isRetriableError(err) {
					log.Warnf("Retriable error getting %s/%s during periodic check (will retry): %v", l.namespace, l.name, err)
				} else {
					log.Errorf("Failed to get %s/%s during periodic check: %v", l.namespace, l.name, err)
				}
				continue
			}

			// Resource exists, check if it's being deleted
			if obj.GetDeletionTimestamp() != nil {
				log.Infof("Resource %s/%s has deletion timestamp during periodic check, removing finalizer", l.namespace, l.name)

				// Save the final workload state
				if err := l.saveGpuWorkload(ctx, obj); err != nil {
					log.Errorf("Failed to save workload %s/%s: %v", l.namespace, l.name, err)
				}

				// Try to remove finalizer
				if err := l.removeFinalizer(ctx, obj); err != nil {
					log.Errorf("Failed to remove finalizer for %s/%s during periodic check: %v", l.namespace, l.name, err)
					// Don't exit, next periodic check will retry
					continue
				}

				// Successfully removed finalizer, exit listener
				log.Infof("Successfully removed finalizer for %s/%s during periodic check, exiting listener", l.namespace, l.name)
				return nil
			}
		}
	}
}

// ensureFinalizer ensures the Finalizer exists with retry mechanism
func (l *Listener) ensureFinalizer(ctx context.Context, obj *unstructured.Unstructured) error {
	// Use retry mechanism to add finalizer
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get latest object before each retry to avoid ResourceVersion conflicts
		latestObj, err := l.getLatestObject(ctx)
		if apierrors.IsNotFound(err) {
			// Object no longer exists, no need to add finalizer
			log.Infof("Resource %s/%s not found during finalizer addition, skipping", l.namespace, l.name)
			return nil
		}
		if err != nil {
			log.Errorf("Failed to get latest object for %s/%s: %v", l.namespace, l.name, err)
			return err
		}

		// Object is being deleted, should not add finalizer
		if latestObj.GetDeletionTimestamp() != nil {
			log.Infof("Resource %s/%s is being deleted, skipping finalizer addition", l.namespace, l.name)
			return nil
		}

		// Check if finalizer already exists
		finalizers := latestObj.GetFinalizers()
		for _, f := range finalizers {
			if f == constant.PrimusLensGpuWorkloadExporterFinalizer {
				log.Debugf("Finalizer already exists for %s/%s", l.namespace, l.name)
				return nil
			}
		}

		// Add finalizer
		latestObj.SetFinalizers(append(finalizers, constant.PrimusLensGpuWorkloadExporterFinalizer))

		// Update object
		_, err = l.client.Dynamic.Resource(l.gvr).
			Namespace(l.namespace).
			Update(ctx, latestObj, metav1.UpdateOptions{})
		if err != nil {
			log.Warnf("Failed to add finalizer for %s/%s (will retry): %v", l.namespace, l.name, err)
		}
		return err
	})

	if retryErr != nil {
		log.Errorf("Failed to add finalizer for %s/%s after retries: %v", l.namespace, l.name, retryErr)
		return retryErr
	}

	log.Infof("Successfully ensured finalizer for %s/%s", l.namespace, l.name)
	return nil
}

// removeFinalizer removes the Finalizer with retry mechanism
func (l *Listener) removeFinalizer(ctx context.Context, obj *unstructured.Unstructured) error {
	// Use retry mechanism to remove finalizer
	retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get latest object before each retry to avoid ResourceVersion conflicts
		latestObj, err := l.getLatestObject(ctx)
		if apierrors.IsNotFound(err) {
			// Object no longer exists, consider success
			log.Infof("Resource %s/%s not found during finalizer removal, considering success", l.namespace, l.name)
			return nil
		}
		if err != nil {
			log.Errorf("Failed to get latest object for %s/%s: %v", l.namespace, l.name, err)
			return err
		}

		// Check if finalizer has already been removed
		hasFinalizer := false
		finalizers := latestObj.GetFinalizers()
		for _, f := range finalizers {
			if f == constant.PrimusLensGpuWorkloadExporterFinalizer {
				hasFinalizer = true
				break
			}
		}

		// If finalizer already removed, no action needed
		if !hasFinalizer {
			log.Infof("Finalizer already removed for %s/%s", l.namespace, l.name)
			return nil
		}

		// Remove finalizer
		newFinalizers := []string{}
		for _, f := range finalizers {
			if f != constant.PrimusLensGpuWorkloadExporterFinalizer {
				newFinalizers = append(newFinalizers, f)
			}
		}
		latestObj.SetFinalizers(newFinalizers)

		// Update object
		_, err = l.client.Dynamic.Resource(l.gvr).
			Namespace(l.namespace).
			Update(ctx, latestObj, metav1.UpdateOptions{})
		if err != nil {
			log.Warnf("Failed to remove finalizer for %s/%s (will retry): %v", l.namespace, l.name, err)
		}
		return err
	})

	if retryErr != nil {
		log.Errorf("Failed to remove finalizer for %s/%s after retries: %v", l.namespace, l.name, retryErr)
		return retryErr
	}

	// Verify finalizer has actually been removed
	if !l.verifyFinalizerRemoved(ctx) {
		err := fmt.Errorf("finalizer removal verification failed for %s/%s", l.namespace, l.name)
		log.Error(err.Error())
		return err
	}

	log.Infof("Successfully removed finalizer for %s/%s", l.namespace, l.name)
	return nil
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
	existGpuWorkload, err := database.GetFacade().GetWorkload().GetGpuWorkloadByUid(ctx, string(obj.GetUID()))
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
		err = database.GetFacade().GetWorkload().CreateGpuWorkload(ctx, existGpuWorkload)

	} else {
		err = database.GetFacade().GetWorkload().UpdateGpuWorkload(ctx, existGpuWorkload)
	}
	if err != nil {
		return err
	}
	return nil
}

func (l *Listener) setWorkloadEnd(ctx context.Context, uid string) error {
	existGpuWorkload, err := database.GetFacade().GetWorkload().GetGpuWorkloadByUid(ctx, uid)
	if err != nil {
		return err
	}
	if existGpuWorkload == nil {
		return nil
	}
	existGpuWorkload.EndAt = existGpuWorkload.UpdatedAt
	return database.GetFacade().GetWorkload().UpdateGpuWorkload(ctx, existGpuWorkload)
}

// isRetriableError determines if an error is retriable
func isRetriableError(err error) bool {
	if err == nil {
		return false
	}
	// Network errors, timeouts, conflicts, service unavailable, etc. should be retried
	return apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsConflict(err) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsInternalError(err) ||
		apierrors.IsTooManyRequests(err)
}

// getLatestObject retrieves the latest version of the resource
func (l *Listener) getLatestObject(ctx context.Context) (*unstructured.Unstructured, error) {
	return l.client.Dynamic.Resource(l.gvr).
		Namespace(l.namespace).
		Get(ctx, l.name, metav1.GetOptions{})
}

// verifyFinalizerRemoved verifies that the finalizer has been removed
func (l *Listener) verifyFinalizerRemoved(ctx context.Context) bool {
	obj, err := l.getLatestObject(ctx)
	if apierrors.IsNotFound(err) {
		// Resource doesn't exist, finalizer must be removed
		return true
	}
	if err != nil {
		log.Warnf("Failed to verify finalizer removal for %s/%s: %v", l.namespace, l.name, err)
		return false
	}

	// Check if finalizer exists
	for _, f := range obj.GetFinalizers() {
		if f == constant.PrimusLensGpuWorkloadExporterFinalizer {
			return false
		}
	}
	return true
}
