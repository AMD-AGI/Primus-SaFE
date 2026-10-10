/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	// SaveImageStagingProject is the built-in Harbor project the containers being saved
	// upload their layers to, one repository per export. It is private.
	SaveImageStagingProject = "save-staging"
	// saveImageStagingRobot names the robot account the containers' upload tokens are
	// minted with. It can push to and pull from the staging project alone: Harbor gives a
	// token the power of the account that minted it, whatever repository it names.
	saveImageStagingRobot = "save-image-staging"
)

// keepEnsuringSaveImageStaging runs ensureSaveImageStaging until it succeeds: Harbor may
// come up after the apiserver.
func (h *ImageHandler) keepEnsuringSaveImageStaging(ctx context.Context) {
	wait := time.Minute
	for {
		err := h.ensureSaveImageStaging(ctx)
		if err == nil {
			return
		}
		klog.Warningf("cannot prepare saving workloads as images in the built-in Harbor (retrying in %s): %v; "+
			"until this succeeds, saving an image is refused", wait, err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(2*wait, 10*time.Minute)
	}
}

// ensureSaveImageStaging makes what saving a workload as an image needs in the built-in
// Harbor, so that no administrator has to: the private staging project, a robot account
// limited to it, and the Secret holding that account's credential, which the resource
// manager mints the containers' upload tokens with. A Secret that exists is never
// changed: it may be an administrator's. Without a built-in Harbor it does nothing; a
// cluster that saves elsewhere configures its own (save_image.clusters).
//
// The staging repositories hold layer blobs only, never an artifact, so a Harbor
// retention policy (which selects artifacts) has nothing to act on there: a layer whose
// image was never put together is reclaimed by Harbor's garbage collection.
func (h *ImageHandler) ensureSaveImageStaging(ctx context.Context) error {
	registry, endpoint, pw, err := h.GetHarborCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to get harbor credentials: %w", err)
	}
	if registry == "" {
		return nil
	}
	return h.ensureStaging(ctx, registry, endpoint, "admin", pw)
}

func (h *ImageHandler) ensureStaging(ctx context.Context, registry, endpoint, user, pw string) error {
	hc := &harborClient{endpoint: endpoint, user: user, password: pw}
	projectID, err := hc.ensurePrivateProject(ctx, SaveImageStagingProject)
	if err != nil {
		return err
	}
	key := client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: common.SaveImageStagingSecretName}
	if err := h.Get(ctx, key, &corev1.Secret{}); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get secret %s: %w", key, err)
	}
	robot, err := hc.ensureRobot(ctx, projectID, SaveImageStagingProject, saveImageStagingRobot)
	if err != nil {
		return err
	}
	secret, err := robotSecret()
	if err != nil {
		return err
	}
	// The Secret is created first: of several apiservers starting at once, only the one
	// that creates it sets the robot's secret, so the two always agree.
	cfg, err := json.Marshal(map[string]any{"auths": map[string]any{registry: map[string]string{
		"username": robot.Name,
		"password": secret,
		"auth":     base64.StdEncoding.EncodeToString([]byte(robot.Name + ":" + secret)),
	}}})
	if err != nil {
		return err
	}
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: key.Namespace,
			Name:      key.Name,
			Labels:    map[string]string{"app.kubernetes.io/managed-by": "primus-safe-apiserver"},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: cfg},
	}
	if err := h.Create(ctx, s); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create secret %s: %w", key, err)
	}
	if err := hc.setRobotSecret(ctx, robot.ID, secret); err != nil {
		if derr := h.Delete(ctx, s); derr != nil && !apierrors.IsNotFound(derr) {
			klog.Warningf("failed to remove secret %s after its robot could not be set: %v", key, derr)
		}
		return err
	}
	klog.Infof("Prepared saving images: private project %s, robot %s limited to it, secret %s",
		SaveImageStagingProject, robot.Name, key)
	return nil
}

// robotSecret is a random robot secret that meets Harbor's rule (8 to 128 characters,
// with an upper and a lower case letter and a digit).
func robotSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b) + "Aa1", nil
}

// harborClient speaks Harbor's v2 API, as an administrator.
type harborClient struct {
	endpoint, user, password string
	scheme                   string // "http" unless a test says otherwise
}

type harborRobot struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

func (c *harborClient) do(ctx context.Context, method, path string, in, out any, want ...int) (int, error) {
	scheme := c.scheme
	if scheme == "" {
		scheme = "http"
	}
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return 0, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, scheme+"://"+c.endpoint+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(c.user, c.password)
	resp, err := newHTTPClientSkipTLS().Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	for _, w := range want {
		if resp.StatusCode == w {
			if out != nil && resp.StatusCode != http.StatusNotFound {
				if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
					return resp.StatusCode, fmt.Errorf("%s %s: %w", method, path, err)
				}
			}
			return resp.StatusCode, nil
		}
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return resp.StatusCode, fmt.Errorf("%s %s: %s %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
}

// ensurePrivateProject creates the project, private, when it is missing, and returns its
// ID. A project that exists is left as it is.
func (c *harborClient) ensurePrivateProject(ctx context.Context, name string) (int64, error) {
	var p struct {
		ProjectID int64 `json:"project_id"`
	}
	code, err := c.do(ctx, http.MethodGet, "/api/v2.0/projects/"+url.PathEscape(name), nil, &p, http.StatusOK, http.StatusNotFound)
	if err != nil {
		return 0, err
	}
	if code == http.StatusOK {
		return p.ProjectID, nil
	}
	if _, err := c.do(ctx, http.MethodPost, "/api/v2.0/projects", map[string]any{
		"project_name": name,
		"metadata":     map[string]string{"public": "false"},
	}, nil, http.StatusCreated, http.StatusConflict); err != nil {
		return 0, err
	}
	if _, err := c.do(ctx, http.MethodGet, "/api/v2.0/projects/"+url.PathEscape(name), nil, &p, http.StatusOK); err != nil {
		return 0, err
	}
	return p.ProjectID, nil
}

// ensureRobot returns the project's robot account of that name, creating it, with push
// and pull on the project's repositories alone and no expiry, when it is missing.
func (c *harborClient) ensureRobot(ctx context.Context, projectID int64, project, name string) (*harborRobot, error) {
	find := func() (*harborRobot, error) {
		var robots []harborRobot
		q := url.Values{"q": {fmt.Sprintf("Level=project,ProjectID=%d", projectID)}, "page_size": {"100"}}
		if _, err := c.do(ctx, http.MethodGet, "/api/v2.0/robots?"+q.Encode(), nil, &robots, http.StatusOK); err != nil {
			return nil, err
		}
		for i := range robots {
			// Harbor names it robot$<project>+<name> (the prefix is configurable).
			if strings.HasSuffix(robots[i].Name, project+"+"+name) {
				return &robots[i], nil
			}
		}
		return nil, nil
	}
	if r, err := find(); err != nil || r != nil {
		return r, err
	}
	var created harborRobot
	code, err := c.do(ctx, http.MethodPost, "/api/v2.0/robots", map[string]any{
		"name":        name,
		"description": "Uploads the layers of workloads being saved as images (primus-safe)",
		"duration":    -1,
		"level":       "project",
		"permissions": []map[string]any{{
			"kind":      "project",
			"namespace": project,
			"access": []map[string]string{
				{"resource": "repository", "action": "push"},
				{"resource": "repository", "action": "pull"},
			},
		}},
	}, &created, http.StatusCreated, http.StatusConflict)
	if err != nil {
		return nil, err
	}
	if code == http.StatusCreated {
		return &created, nil
	}
	r, err := find()
	if err == nil && r == nil {
		err = fmt.Errorf("robot %s of project %s exists but cannot be found", name, project)
	}
	return r, err
}

// setRobotSecret gives the robot account the secret.
func (c *harborClient) setRobotSecret(ctx context.Context, id int64, secret string) error {
	_, err := c.do(ctx, http.MethodPatch, fmt.Sprintf("/api/v2.0/robots/%d", id), map[string]string{"secret": secret}, nil, http.StatusOK)
	return err
}
