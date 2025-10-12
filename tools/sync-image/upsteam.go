package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/containers/image/v5/types"
)

const (
	importImageUpsteamPath = "http://%s/api/v1/images:import/%s/progress"
)

const (
	DefaultTimeout = 30 * time.Second
)

var (
	client = NewClient()
)

func NewClient() *http.Client {
	return &http.Client{
		Timeout: DefaultTimeout,
	}
}

type UpstreamEvent struct {
	Data              map[string]types.ProgressProperties `json:"data"`
	SyncLayerCount    int                                 `json:"syncLayerCount"`
	ComplexLayerCount int                                 `json:"complexLayerCount"`
	SkipLayerCount    int                                 `json:"skipLayerCount"`
}

func upstreamData(domain, imageName string, data *UpstreamEvent) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	encodedImageName := base64.URLEncoding.EncodeToString([]byte(imageName))
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf(importImageUpsteamPath, domain, encodedImageName), bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
