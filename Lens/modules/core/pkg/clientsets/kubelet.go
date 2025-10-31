package clientsets

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"sync"

	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/go-resty/resty/v2"
	corev1 "k8s.io/api/core/v1"
	statsapi "k8s.io/kubelet/pkg/apis/stats/v1alpha1"
)

const (
	kubeletStatsApi = "/stats/summary"
	podsApi         = "/pods"
)

func NewClient(kubeletAddress string) (*Client, error) {
	path := os.Getenv("KUBELET_TOKEN_FILE")
	if path == "" {
		path = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	}
	tokenBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read token file failed.Path %s:%w", path, err)
	}
	restyC := resty.New().SetBaseURL(kubeletAddress).SetHeader("Authorization", fmt.Sprintf("Bearer %s", string(tokenBytes)))
	restyC.SetTLSClientConfig(&tls.Config{InsecureSkipVerify: true})
	return &Client{
		kubeletApi: restyC,
	}, nil
}

type Client struct {
	address    string
	kubeletApi *resty.Client
}

func (s *Client) GetRestyClient() *resty.Client {
	return s.kubeletApi.Clone()
}

func (s *Client) GetKubeletStats(ctx context.Context) *statsapi.Summary {
	resp, err := s.kubeletApi.R().SetResult(&statsapi.Summary{}).Get(kubeletStatsApi)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).WithError(err).Errorln("Failed to get kubelet stats")
		return nil
	}
	if resp.StatusCode() != http.StatusOK {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get kubelet stats, status code: %d", resp.StatusCode())
		return nil
	}
	return resp.Result().(*statsapi.Summary)
}

func (s *Client) GetKubeletPods(ctx context.Context) (*corev1.PodList, error) {
	resp, err := s.kubeletApi.R().SetResult(&corev1.PodList{}).Get(podsApi)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).WithError(err).Errorln("Failed to get kubelet pods")
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get kubelet pods, status code: %d.Resp %s", resp.StatusCode(), resp.String())
		return nil, err
	}
	return resp.Result().(*corev1.PodList), nil
}

func (s *Client) GetKubeletPodMap(ctx context.Context) (map[string]corev1.Pod, error) {
	podList, err := s.GetKubeletPods(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]corev1.Pod{}
	for i := range podList.Items {
		pod := podList.Items[i]
		result[string(pod.UID)] = pod
	}
	return result, nil
}

var kubeletClients = map[string]*Client{}

var kubeletLock sync.Mutex

func GetOrInitKubeletClient(nodeName, address string) (*Client, error) {
	kubeletLock.Lock()
	defer kubeletLock.Unlock()

	existing, ok := kubeletClients[nodeName]
	if ok {
		if existing.address == address {
			return existing, nil
		}
		log.GlobalLogger().WithField("node", nodeName).Warningf("Kubelet address changed: old=%s, new=%s. Reinitializing client.", existing.address, address)
	}

	newClient, err := NewClient(address)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubelet client for node %s: %w", nodeName, err)
	}

	newClient.address = address

	kubeletClients[nodeName] = newClient
	return newClient, nil
}
