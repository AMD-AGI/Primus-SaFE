/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	_, endpoint, password, err := h.GetHarborCredentials(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get harbor credentials: %w", err)
	}
	username := "admin"
	harborStats, err := h.getHarborStats(ctx, endpoint, username, password)
	if err != nil {
		return nil, fmt.Errorf("failed to get harbor health: %w", err)
	}
	return harborStats, nil
}

func (h *ImageHandler) initHarbor(ctx context.Context) error {
	harborHost, endpoint, password, err := h.GetHarborCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to get harbor credentials: %w", err)
	}
	if harborHost == "" {
		// No Harbor
		return nil
	}
	username := "admin"
	if err := h.ensureHarborProject(ctx, endpoint, username, password, SyncImageProject); err != nil {
		return fmt.Errorf("failed to ensure harbor project: %w", err)
	}
	err = h.setDefaultImageRegistry(ctx, harborHost, username, password)
	if err != nil {
		return err
	}
	return nil
}

func (h *ImageHandler) setDefaultImageRegistry(ctx context.Context, url, username, password string) error {
	req := &CreateRegistryRequest{
		Name:     "Builtin",
		Url:      url,
		UserName: username,
		Password: password,
		Default:  true,
	}
	exist, err := h.dbClient.GetRegistryInfoByUrl(ctx, url)
	if err != nil {
		return fmt.Errorf("failed to get existing registry info: %w", err)
	}
	newInfo, err := h.cvtCreateRegistryRequestToRegistryInfo(req)
	if err != nil {
		return fmt.Errorf("failed to convert registry request to model: %w", err)
	}
	if exist != nil {
		newInfo.ID = exist.ID
		newInfo.UpdatedAt = time.Now()
	} else {
		newInfo.CreatedAt = time.Now()
		newInfo.UpdatedAt = time.Now()
	}
	err = h.dbClient.UpsertRegistryInfo(ctx, newInfo)
	if err != nil {
		return fmt.Errorf("failed to update existing registry info: %w", err)
	}
	err = h.refreshImageImportSecrets(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (h *ImageHandler) getHarborStats(ctx context.Context, harborHost, username, password string) (*HarborStat, error) {
	var health HarborHealth
	if err := h.harborRequest(ctx, harborHost, "/api/v2.0/health", username, password, &health); err != nil {
		return nil, err
	}

	var stats HarborStatistics
	if err := h.harborRequest(ctx, harborHost, "/api/v2.0/statistics", username, password, &stats); err != nil {
		return nil, err
	}
	return &HarborStat{
		HarborHealth:     health,
		HarborStatistics: stats,
	}, nil
}

func (h *ImageHandler) GetHarborCredentials(ctx context.Context) (domain, endpoint, password string, err error) {
	const (
		namespace         = "harbor"
		configMapName     = "harbor-core"
		secretName        = "harbor-core"
		serviceName       = "harbor-core"
		configKeyEndpoint = "EXT_ENDPOINT"
		secretKeyPassword = "HARBOR_ADMIN_PASSWORD"
	)

	var cm corev1.ConfigMap
	if err = h.Get(ctx, client.ObjectKey{Namespace: namespace, Name: configMapName}, &cm); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return "", "", "", nil
		}
		return "", "", "", fmt.Errorf("failed to get configmap %s/%s: %w", namespace, configMapName, err)
	}

	endpoint, ok := cm.Data[configKeyEndpoint]
	if !ok {
		return "", "", "", fmt.Errorf("configmap %s/%s missing key %s", namespace, configMapName, configKeyEndpoint)
	}

	var sec corev1.Secret
	if err = h.Get(ctx, client.ObjectKey{Namespace: namespace, Name: secretName}, &sec); err != nil {
		return "", "", "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretName, err)
	}

	pwBytes, ok := sec.Data[secretKeyPassword]
	if !ok {
		return "", "", "", fmt.Errorf("secret %s/%s missing key %s", namespace, secretName, secretKeyPassword)
	}

	password = string(pwBytes)
	domain = endpoint
	if strings.HasPrefix(domain, "https://") {
		domain = strings.TrimPrefix(endpoint, "https://")
	} else if strings.HasPrefix(domain, "http://") {
		domain = strings.TrimPrefix(endpoint, "http://")
	}
	return domain, fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, namespace), password, nil
}
func (h *ImageHandler) ensureHarborProject(ctx context.Context, harborHost, username, password, projectName string) error {
	var project struct {
		Name      string            `json:"name"`
		Metadata  map[string]string `json:"metadata"`
		ProjectID int               `json:"project_id"`
		Public    bool              `json:"public"`
	}

	checkPath := fmt.Sprintf("/api/v2.0/projects/%s", projectName)
	err := h.harborRequest(ctx, harborHost, checkPath, username, password, &project)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			createPayload := map[string]any{
				"project_name": projectName,
				"metadata": map[string]string{
					"public": "true",
				},
			}
			return h.harborPost(ctx, harborHost, "/api/v2.0/projects", username, password, createPayload)
		}
		return fmt.Errorf("failed to check project: %w", err)
	}

	if !project.Public {
		updatePayload := map[string]any{
			"metadata": map[string]string{
				"public": "true",
			},
		}
		updatePath := fmt.Sprintf("/api/v2.0/projects/%s", projectName)
		return h.harborPut(ctx, harborHost, updatePath, username, password, updatePayload)
	}

	return nil
}

func (h *ImageHandler) harborPost(ctx context.Context, harborHost, path, username, password string, payload any) error {
	return h.harborDo(ctx, harborHost, path, username, password, http.MethodPost, payload, []int{http.StatusCreated, http.StatusOK})
}

func (h *ImageHandler) harborPut(ctx context.Context, harborHost, path, username, password string, payload any) error {
	return h.harborDo(ctx, harborHost, path, username, password, http.MethodPut, payload, []int{http.StatusOK})
}

func (h *ImageHandler) harborDo(ctx context.Context, harborHost, path, username, password, method string, payload any, allowedStatus []int) error {
	url := fmt.Sprintf("http://%s%s", harborHost, path)

	var bodyReader io.Reader
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal payload failed: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	client := newHTTPClientSkipTLS()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s request failed: %w", method, err)
	}
	defer resp.Body.Close()

	for _, code := range allowedStatus {
		if resp.StatusCode == code {
			return nil
		}
	}

	respBody, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s unexpected status: %s - %s", method, resp.Status, string(respBody))
}

func (h *ImageHandler) harborRequest(ctx context.Context, harborHost, path, username, password string, result any) error {
	url := fmt.Sprintf("http://%s%s", harborHost, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	client := newHTTPClientSkipTLS()
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

func newHTTPClientSkipTLS() *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}
}
