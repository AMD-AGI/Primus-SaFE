package clientsets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/k8sUtil"
	"github.com/go-resty/resty/v2"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gpusAPI             = "/v1/gpus"
	gpuDriverVersionAPI = "/v1/gpuDriverVersion"
	gpuCardMetricsAPI   = "/v1/cardMetrics"
	rdmaDevicesAPI      = "/v1/rdma"
)

type NodeExporterClient struct {
	address string
	api     *resty.Client
}

func NewNodeExporterClient(address string) *NodeExporterClient {
	restyC := resty.New().SetBaseURL(address)
	return &NodeExporterClient{
		address: address,
		api:     restyC,
	}
}

func (c *NodeExporterClient) GetRestyClient() *resty.Client {
	return c.api.Clone()
}

func (c *NodeExporterClient) GetGPUs(ctx context.Context) ([]model.GPUInfo, error) {
	resp, err := c.api.R().
		SetContext(ctx).
		SetResult(&rest.Response{}).
		Get(gpusAPI)

	if err != nil {
		return nil, fmt.Errorf("failed to get GPUs: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode(), resp.String())
	}
	resultResp, ok := resp.Result().(*rest.Response)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp.Result())
	}
	if resultResp.Meta.Code != rest.CodeSuccess {
		return nil, fmt.Errorf("unexpected response code: %d, response: %s", resultResp.Meta.Code, resultResp.Meta.Message)
	}
	gpus := []model.GPUInfo{}
	jsonStr, err := json.Marshal(resultResp.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonStr, &gpus)
	if err != nil {
		return nil, err
	}
	return gpus, nil
}

func (c *NodeExporterClient) GetDriverVersion(ctx context.Context) (string, error) {
	resp, err := c.api.R().
		SetContext(ctx).
		SetResult(&rest.Response{}).
		Get(gpuDriverVersionAPI)

	if err != nil {
		return "", fmt.Errorf("failed to get GPU driver version: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode(), resp.String())
	}
	resultResp, ok := resp.Result().(*rest.Response)
	if !ok {
		return "", fmt.Errorf("unexpected response type: %T", resp.Result())
	}
	if resultResp.Meta.Code != rest.CodeSuccess {
		return "", fmt.Errorf("unexpected response code: %d, response: %s", resultResp.Meta.Code, resultResp.Meta.Message)
	}
	version, ok := resultResp.Data.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type: %T", resultResp.Data.(interface{}))
	}
	return version, nil
}

func (c *NodeExporterClient) GetCardMetrics(ctx context.Context) ([]model.CardMetrics, error) {
	resp, err := c.api.R().
		SetContext(ctx).
		SetResult(&rest.Response{}).
		Get(gpuCardMetricsAPI)

	if err != nil {
		return nil, fmt.Errorf("failed to get GPU driver version: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode(), resp.String())
	}
	resultResp, ok := resp.Result().(*rest.Response)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp.Result())
	}
	if resultResp.Meta.Code != rest.CodeSuccess {
		return nil, fmt.Errorf("unexpected response code: %d, response: %s", resultResp.Meta.Code, resultResp.Meta.Message)
	}
	cardMetrics := []model.CardMetrics{}
	jsonStr, err := json.Marshal(resultResp.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonStr, &cardMetrics)
	if err != nil {
		return nil, err
	}
	return cardMetrics, nil
}

func (c *NodeExporterClient) GetRdmaDevices(ctx context.Context) ([]model.RDMADevice, error) {
	resp, err := c.api.R().
		SetContext(ctx).
		SetResult(&rest.Response{}).
		Get(rdmaDevicesAPI)

	if err != nil {
		return nil, fmt.Errorf("failed to get GPUs: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode(), resp.String())
	}
	resultResp, ok := resp.Result().(*rest.Response)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", resp.Result())
	}
	if resultResp.Meta.Code != rest.CodeSuccess {
		return nil, fmt.Errorf("unexpected response code: %d, response: %s", resultResp.Meta.Code, resultResp.Meta.Message)
	}
	rdmaDevices := []model.RDMADevice{}
	jsonStr, err := json.Marshal(resultResp.Data)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(jsonStr, &rdmaDevices)
	if err != nil {
		return nil, err
	}
	return rdmaDevices, nil
}

var nodeExportersClients = map[string]*NodeExporterClient{}

var nodeExportersLock sync.Mutex

func GetOrInitNodeExportersClient(ctx context.Context, nodeName string, k8sClient client.Client) (*NodeExporterClient, error) {
	selector := labels.SelectorFromSet(map[string]string{
		"app": "primus-lens-node-exporter",
	})

	pod, err := k8sUtil.GetTargetPod(ctx, k8sClient, "primus-lens", selector, nodeName)
	if err != nil {
		return nil, err
	}
	if !k8sUtil.IsPodRunning(pod) {
		return nil, fmt.Errorf("pod %s/%s is not running", pod.Namespace, pod.Name)
	}
	nodeExportersLock.Lock()
	defer nodeExportersLock.Unlock()
	address := fmt.Sprintf("http://%s:8989", pod.Status.PodIP)
	existing, ok := nodeExportersClients[nodeName]
	if ok {
		if existing.address == address {
			return existing, nil
		}
		log.GlobalLogger().WithField("node", nodeName).Warningf("nodeExporters address changed: old=%s, new=%s. Reinitializing client.", existing.address, address)
	}

	newClient := NewNodeExporterClient(address)
	newClient.address = address

	nodeExportersClients[nodeName] = newClient
	return newClient, nil
}
