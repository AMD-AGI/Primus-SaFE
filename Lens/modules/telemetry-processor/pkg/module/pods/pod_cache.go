package pods

import (
	"context"
	"strconv"
	"time"

	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
)

var (
	nodeDeviceUidCache  = map[string]map[string]map[string]string{}   // nodeName->deviceLabel->deviceName-> deviceUid
	nodeDevicePodCache  = map[string]map[string]map[string][]string{} // nodeName->deviceLabel->deviceName->podName&podUid
	podWorkloadCache    = map[string][][]string{}                     // podName -> workloadName&workloadUid
	podUidWorkloadCache = map[string][][]string{}                     // podUid -> workloadName&workloadUid
	podNameUidCache     = map[string]string{}                         // podName -> podUid
)

func GetNodeDevicePodCache() map[string]map[string]map[string][]string {
	return nodeDevicePodCache
}

func GetPodWorkloadCache() map[string][][]string {
	return podWorkloadCache
}

func GetPodUidWorkloadCache() map[string][][]string {
	return podUidWorkloadCache
}

func StartRefreshCaches(ctx context.Context) {
	go runLoadDevicePodCache(ctx)
	go runLoadPodWorkloadCache(ctx)

	log.Infof("started device pod cache and pod workload cache loaders")
}

func getName(labels []prompb.Label) string {
	for _, label := range labels {
		if label.Name == "__name__" {
			return label.Value
		}
	}
	return ""
}

func GetPodLabelValue(labels []prompb.Label) (podName string, podUid string) {
	labelNames := map[string]struct{}{}
	labelValues := map[string]string{}
	for _, label := range labels {
		labelNames[label.Name] = struct{}{}
		labelValues[label.Name] = label.Value

	}

	if _, ok := labelNames[constant.PrimusLensNodeLabelName]; !ok {
		return "", ""
	}
	node := labelValues[constant.PrimusLensNodeLabelName]
	if node == "" {
		return "", ""
	}
	if _, ok := nodeDevicePodCache[node]; !ok {
		return "", ""
	}
	for deviceLabel, deviceMap := range nodeDevicePodCache[node] {
		metricDevice := labelValues[deviceLabel]
		if _, ok := deviceMap[metricDevice]; !ok {
			continue
		}
		if _, ok := deviceMap[metricDevice]; !ok {
			continue
		}
		result := deviceMap[metricDevice]
		if len(result) < 2 {
			log.Errorf("device pod cache for node %s, device label %s, device %s has less than 2 elements: %v", node, deviceLabel, metricDevice, result)
			continue
		}
		podName = result[0]
		podUid = result[1]
		return
	}
	// filter kube state metrics
	podName = labelValues["pod"]
	podUid = labelValues["uid"]
	if podName != "" && podUid != "" {
		return podName, podUid
	}
	// filter kubelet metrics
	if podName != "" {
		return podName, "unknown"
	}
	return "", ""
}

func GetWorkloadsByPodName(podName string) [][]string {
	if workloads, ok := podWorkloadCache[podName]; ok {
		return workloads
	}
	return nil
}

func GetWorkloadsByPodUid(podUid string) [][]string {
	if workloads, ok := podUidWorkloadCache[podUid]; ok {
		return workloads
	}
	return nil
}

func runLoadDevicePodCache(ctx context.Context) {
	for {
		err := loadDevicePodCache(ctx)
		if err != nil {
			log.Errorf("failed to load device pod cache: %s", err)
		}
		log.Infof("device pod cache loaded successfully")
		select {
		case <-ctx.Done():
			log.Infof("stopping device pod cache loader")
			return
		default:
			// continue loading
			time.Sleep(20 * time.Second) // Adjust the interval as needed
		}
	}
}
func getLabelByDeviceType(deviceType string) string {
	switch deviceType {
	case constant.DeviceTypeGPU:
		return "gpu_id"
	case constant.DeviceTypeIB:
		return "device"
	case constant.DeviceTypeRDMA:
		return "device"
	case "ASIC":
		return "asic"
	default:
		return "unknown"
	}
}

func GetDeviceKey(device *dbModel.NodeContainerDevices) string {
	switch device.DeviceType {
	case constant.DeviceTypeGPU:
		return strconv.Itoa(int(device.DeviceNo))
	case constant.DeviceTypeIB:
		return device.DeviceName
	case constant.DeviceTypeRDMA:
		return device.DeviceName
	case "ASIC":
		return device.DeviceName
	default:
		return "unknown"
	}
}

func loadDevicePodCache(ctx context.Context) error {
	// Use current cluster name for cache loading in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	newNodeDevicePodCache := map[string]map[string]map[string][]string{}
	runningGpuPod, err := database.GetFacadeForCluster(clusterName).GetPod().ListActiveGpuPods(ctx)
	if err != nil {
		log.Errorf("cannot load running gpu pods: %s", err)
		return err
	}
	for _, pod := range runningGpuPod {
		runningContainer, err := database.GetFacadeForCluster(clusterName).GetContainer().ListRunningContainersByPodUid(ctx, pod.UID)
		if err != nil {
			log.Errorf("cannot load running containers for pod %s: %s", pod.Name, err)
			continue
		}
		for _, container := range runningContainer {
			devices, err := database.GetFacadeForCluster(clusterName).GetContainer().ListContainerDevicesByContainerId(ctx, container.ContainerID)
			if err != nil {
				log.Errorf("cannot load devices for container %s: %s", container.ContainerID, err)
				continue
			}
			for _, device := range devices {
				deviceLabel := getLabelByDeviceType(device.DeviceType)
				if _, ok := newNodeDevicePodCache[pod.NodeName]; !ok {
					newNodeDevicePodCache[pod.NodeName] = map[string]map[string][]string{}
				}
				if _, ok := newNodeDevicePodCache[pod.NodeName][deviceLabel]; !ok {
					newNodeDevicePodCache[pod.NodeName][deviceLabel] = map[string][]string{}
				}
				newNodeDevicePodCache[pod.NodeName][deviceLabel][GetDeviceKey(device)] = []string{pod.Name, pod.UID}
			}
		}
	}
	nodeDevicePodCache = newNodeDevicePodCache
	log.Infof("loaded device pod cache with %d nodes", len(nodeDevicePodCache))
	return nil
}

func runLoadPodWorkloadCache(ctx context.Context) {
	for {
		err := loadPodWorkloadCache(ctx)
		if err != nil {
			log.Errorf("failed to load pod workload cache: %s", err)
		}
		log.Infof("pod workload cache loaded successfully")
		select {
		case <-ctx.Done():
			log.Infof("stopping pod workload cache loader")
			return
		default:
			// continue loading
			time.Sleep(20 * time.Second) // Adjust the interval as needed
		}
	}
}

func loadPodWorkloadCache(ctx context.Context) error {
	// Use current cluster name for cache loading in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()
	newPodWorkloadCache := map[string][][]string{}
	newPodUidWorkloadCache := map[string][][]string{}
	runningWorkload, err := database.GetFacadeForCluster(clusterName).GetWorkload().ListRunningWorkload(ctx)
	if err != nil {
		log.Errorf("cannot load running workloads: %s", err)
		return err
	}
	for _, w := range runningWorkload {
		pods, err := workload.GetActivePodsByWorkloadUid(ctx, clusterName, w.UID)
		if err != nil {
			log.Errorf("cannot load pods for workload %s: %s", w.Name, err)
			continue
		}
		for _, pod := range pods {
			if _, ok := newPodWorkloadCache[pod.Name]; !ok {
				newPodWorkloadCache[pod.Name] = [][]string{}
			}
			newPodWorkloadCache[pod.Name] = append(newPodWorkloadCache[pod.Name], []string{w.Name, w.UID})
			if _, ok := newPodUidWorkloadCache[pod.UID]; !ok {
				newPodUidWorkloadCache[pod.UID] = [][]string{}
			}
			newPodUidWorkloadCache[pod.UID] = append(newPodUidWorkloadCache[pod.UID], []string{w.Name, w.UID})
		}
	}
	podWorkloadCache = newPodWorkloadCache
	podUidWorkloadCache = newPodUidWorkloadCache
	log.Infof("loaded pod workload cache with %d pods", len(podWorkloadCache))
	log.Infof("loaded pod workload cache with %d pods", len(newPodWorkloadCache))
	return nil
}
