package gpu_pod

import (
	"context"
	"sync"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GpuPodJob struct {
}

func (g *GpuPodJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) error {
	activePods, err := database.GetFacade().GetPod().ListActiveGpuPods(ctx)
	if err != nil {
		log.Errorf("list active gpu pods: %v", err)
		return err
	}
	wg := &sync.WaitGroup{}
	for i := range activePods {
		dbPod := activePods[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := g.checkForSinglePod(ctx, dbPod, clientSets)
			if err != nil {
				log.Errorf("Failed to check pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (g *GpuPodJob) Schedule() string {
	return "@every 5s"
}

func (g *GpuPodJob) checkForSinglePod(ctx context.Context, dbPod *dbModel.GpuPods, clientSets *clientsets.K8SClientSet) error {
	pod := &corev1.Pod{}
	chaged := false
	err := clientSets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{
		Namespace: dbPod.Namespace,
		Name:      dbPod.Name,
	}, pod)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			log.Errorf("Failed to get pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			return err
		}
		// Pod not found, mark it as deleted
		dbPod.Phase = string(corev1.PodSucceeded)
		dbPod.Running = false
		dbPod.Deleted = true
		chaged = true
	} else {
		if dbPod.Phase != string(pod.Status.Phase) {
			log.Infof("Pod %s/%s phase changed from %s to %s", dbPod.Namespace, dbPod.Name, dbPod.Phase, pod.Status.Phase)
			dbPod.Phase = string(pod.Status.Phase)
		}
	}
	if chaged {
		err = database.GetFacade().GetPod().UpdateGpuPods(ctx, dbPod)
		if err != nil {
			log.Errorf("Failed to update pod %s/%s: %v", dbPod.Namespace, dbPod.Name, err)
			return err
		}
		log.Infof("Updated pod %s/%s phase to %s", dbPod.Namespace, dbPod.Name, dbPod.Phase)
	}
	return nil
}
