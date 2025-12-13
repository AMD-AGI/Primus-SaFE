package processtree

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// KubeletReader reads pod information from kubelet API
// Note: Currently simplified - can be extended to use kubelet stats API
type KubeletReader struct{}

// KubeletPodInfo represents pod information from kubelet
type KubeletPodInfo struct {
	Name       string
	Namespace  string
	UID        string
	NodeName   string
	Phase      string
	Containers []string
}

// NewKubeletReader creates a new kubelet reader
func NewKubeletReader() (*KubeletReader, error) {
	return &KubeletReader{}, nil
}

// GetPodInfo retrieves pod information
// Currently returns minimal info - can be extended to query kubelet stats API
func (r *KubeletReader) GetPodInfo(ctx context.Context, namespace, name string) (*KubeletPodInfo, error) {
	// For now, return basic info
	// This can be extended to query kubelet stats API for additional metadata
	log.Debugf("Kubelet reader: getting pod info for %s/%s", namespace, name)

	info := &KubeletPodInfo{
		Name:      name,
		Namespace: namespace,
	}

	return info, nil
}

// GetPodByUID retrieves pod information by UID
func (r *KubeletReader) GetPodByUID(ctx context.Context, podUID string) (*KubeletPodInfo, error) {
	// For now, return basic info with UID
	// This can be extended to query kubelet stats API
	log.Debugf("Kubelet reader: getting pod info by UID %s", podUID)

	info := &KubeletPodInfo{
		UID: podUID,
	}

	return info, nil
}
