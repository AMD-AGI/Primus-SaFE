package image_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

type HealthComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type HarborStat struct {
	HarborHealth
	HarborStatistics
}

type HarborHealth struct {
	Status     string            `json:"status"`
	Components []HealthComponent `json:"components"`
}

type HarborStatistics struct {
	PrivateProjectCount int   `json:"private_project_count"`
	PublicProjectCount  int   `json:"public_project_count"`
	PrivateRepoCount    int   `json:"private_repo_count"`
	PublicRepoCount     int   `json:"public_repo_count"`
	TotalStorage        int64 `json:"total_storage"`
}

func (h *ImageHandler) GetHarborStats(ctx *gin.Context) (*HarborStat, error) {
	_, password, err := h.GetHarborCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get harbor credentials: %w", err)
	}
	harborHost := "harbor-core.harbor.svc.cluster.local"
	username := "admin"

	harborStats, err := h.getHarborStats(ctx, harborHost, username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to get harbor health: %w", err)
	}
	return harborStats, nil
}

func (h *ImageHandler) getHarborStats(ctx context.Context, harborHost, username, password string) (*HarborStat, error) {
	var health HarborHealth
	if err := h.harborRequest(ctx, harborHost, "/api/v2.0/health", username, password, &health); err != nil {
		return nil, err
	}

	// 2️⃣ 获取 Harbor 整体磁盘使用情况
	var stats HarborStatistics
	if err := h.harborRequest(ctx, harborHost, "/api/v2.0/statistics", username, password, &stats); err != nil {
		return nil, err
	}
	return &HarborStat{
		HarborHealth:     health,
		HarborStatistics: stats,
	}, nil
}

func (h *ImageHandler) harborRequest(ctx context.Context, harborHost, path, username, password string, result any) error {
	url := fmt.Sprintf("http://%s%s", harborHost, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	// 添加认证
	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(result)
}

func (h *ImageHandler) GetHarborCredentials(ctx context.Context) (endpoint, password string, err error) {
	const (
		namespace         = "harbor"
		configMapName     = "harbor-core"
		secretName        = "harbor-core"
		configKeyEndpoint = "EXT_ENDPOINT"
		secretKeyPassword = "HARBOR_ADMIN_PASSWORD"
	)

	// 获取 ConfigMap
	var cm corev1.ConfigMap
	if err = h.Get(ctx, client.ObjectKey{Namespace: namespace, Name: configMapName}, &cm); err != nil {
		return "", "", fmt.Errorf("failed to get configmap %s/%s: %w", namespace, configMapName, err)
	}

	endpoint, ok := cm.Data[configKeyEndpoint]
	if !ok {
		return "", "", fmt.Errorf("configmap %s/%s missing key %s", namespace, configMapName, configKeyEndpoint)
	}

	// 获取 Secret
	var sec corev1.Secret
	if err = h.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &sec); err != nil {
		return "", "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	pwBytes, ok := sec.Data[secretKeyPassword]
	if !ok {
		return "", "", fmt.Errorf("secret %s/%s missing key %s", namespace, secretName, secretKeyPassword)
	}

	password = string(pwBytes)
	domain := endpoint
	if strings.HasPrefix(domain, "https://") {
		domain = strings.TrimPrefix(endpoint, "https://")
	} else if strings.HasPrefix(domain, "http://") {
		domain = strings.TrimPrefix(endpoint, "http://")
	}
	return domain, password, nil
}
