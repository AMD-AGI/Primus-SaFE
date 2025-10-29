package fault

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func GetFaultyNodes(ctx context.Context, clientsets *clientsets.K8SClientSet, nodes []string) ([]string, error) {
	faulty := []string{}
	for _, nodeName := range nodes {
		node := corev1.Node{}
		err := clientsets.ControllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, &node)
		if err != nil {
			log.Errorf("Get node %s error: %v", nodeName, err)
			return nil, err
		}
		if len(node.Spec.Taints) > 0 {
			faulty = append(faulty, nodeName)
			continue
		}
		if !k8sUtil.NodeReady(node) {
			faulty = append(faulty, nodeName)
		}
	}
	return faulty, nil
}
