/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	testifyassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

// TestCvtImageToFlatResponse tests conversion from model.Image to flat Image response
func TestCvtImageToFlatResponse(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		images   []*model.Image
		validate func(*testing.T, []Image)
	}{
		{
			name: "single image conversion",
			images: []*model.Image{
				{
					ID:          1,
					Tag:         "docker.io/library/nginx:latest",
					Description: "Nginx web server",
					CreatedBy:   "user1",
					CreatedAt:   now,
				},
			},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 1)
				assert.Equal(t, int32(1), result[0].Id)
				assert.Equal(t, "docker.io/library/nginx:latest", result[0].Tag)
				assert.Equal(t, "Nginx web server", result[0].Description)
				assert.Equal(t, "user1", result[0].CreatedBy)
				assert.Equal(t, now.Unix(), result[0].CreatedAt)
			},
		},
		{
			name: "multiple images conversion",
			images: []*model.Image{
				{
					ID:          1,
					Tag:         "harbor.example.com/project/app:v1.0",
					Description: "App v1.0",
					CreatedBy:   "admin",
					CreatedAt:   now,
				},
				{
					ID:          2,
					Tag:         "harbor.example.com/project/app:v2.0",
					Description: "App v2.0",
					CreatedBy:   "admin",
					CreatedAt:   now.Add(time.Hour),
				},
			},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 2)
				assert.Equal(t, int32(1), result[0].Id)
				assert.Equal(t, int32(2), result[1].Id)
				assert.Equal(t, "App v1.0", result[0].Description)
				assert.Equal(t, "App v2.0", result[1].Description)
			},
		},
		{
			name:   "empty images list",
			images: []*model.Image{},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 0)
			},
		},
		{
			name: "image without description",
			images: []*model.Image{
				{
					ID:        10,
					Tag:       "gcr.io/project/image:tag",
					CreatedBy: "user",
					CreatedAt: now,
				},
			},
			validate: func(t *testing.T, result []Image) {
				assert.Len(t, result, 1)
				assert.Empty(t, result[0].Description)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtImageToFlatResponse(tt.images)
			tt.validate(t, result)
		})
	}
}

// TestGenerateImportImageJobName tests generation of import image job names
func TestGenerateImportImageJobName(t *testing.T) {
	tests := []struct {
		name    string
		imageId int32
	}{
		{
			name:    "basic image ID",
			imageId: 123,
		},
		{
			name:    "large image ID",
			imageId: 999999,
		},
		{
			name:    "image ID 1",
			imageId: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateImportImageJobName(tt.imageId)

			// Job name should start with prefix "imptimg-"
			assert.Contains(t, result, "imptimg-")

			// Should be non-empty and have reasonable length
			assert.NotEmpty(t, result)
			assert.Greater(t, len(result), 20) // "imptimg-" + ID + "-" + 16-char hash

			// Verify format: imptimg-{id}-{16-hex-digits}
			assert.Regexp(t, `^imptimg-\d+-[0-9a-f]{16}$`, result)

			// Generate again should produce different result (due to timestamp)
			time.Sleep(1 * time.Millisecond)
			result2 := generateImportImageJobName(tt.imageId)
			assert.NotEqual(t, result, result2, "Different calls should produce different hashes")
		})
	}
}

// TestGenerateTargetImageName tests generation of target image names
func TestGenerateTargetImageName(t *testing.T) {
	tests := []struct {
		name               string
		targetRegistryHost string
		sourceImage        string
		expectedContains   []string
		wantErr            bool
	}{
		{
			name:               "valid docker.io image",
			targetRegistryHost: "harbor.example.com",
			sourceImage:        "docker.io/library/nginx:latest",
			expectedContains:   []string{"harbor.example.com", "sync", "library/nginx:latest"},
			wantErr:            false,
		},
		{
			name:               "valid gcr.io image",
			targetRegistryHost: "my-registry.io",
			sourceImage:        "gcr.io/project/app:v1.0",
			expectedContains:   []string{"my-registry.io", "sync", "project/app:v1.0"},
			wantErr:            false,
		},
		{
			name:               "invalid source image - no registry",
			targetRegistryHost: "harbor.example.com",
			sourceImage:        "nginx:latest",
			expectedContains:   nil,
			wantErr:            true,
		},
		{
			name:               "invalid source image - empty",
			targetRegistryHost: "harbor.example.com",
			sourceImage:        "",
			expectedContains:   nil,
			wantErr:            true,
		},
		{
			name:               "complex image path",
			targetRegistryHost: "internal-harbor.com",
			sourceImage:        "quay.io/organization/team/app:v2.3.1",
			expectedContains:   []string{"internal-harbor.com", "sync", "organization/team/app:v2.3.1"},
			wantErr:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateTargetImageName(tt.targetRegistryHost, tt.sourceImage)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}

// TestGenerateAuthValue tests generation of authentication values
func TestGenerateAuthValue(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		validate func(*testing.T, string)
	}{
		{
			name:     "basic auth",
			username: "admin",
			password: "password123",
			validate: func(t *testing.T, result string) {
				// Decode and verify
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, "admin:password123", string(decoded))
			},
		},
		{
			name:     "username with special characters",
			username: "user@example.com",
			password: "pass",
			validate: func(t *testing.T, result string) {
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, "user@example.com:pass", string(decoded))
			},
		},
		{
			name:     "empty credentials",
			username: "",
			password: "",
			validate: func(t *testing.T, result string) {
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, ":", string(decoded))
			},
		},
		{
			name:     "password with special characters",
			username: "user",
			password: "yourword",
			validate: func(t *testing.T, result string) {
				decoded, err := base64.URLEncoding.DecodeString(result)
				assert.NoError(t, err)
				assert.Equal(t, "user:yourword", string(decoded))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateAuthValue(tt.username, tt.password)
			assert.NotEmpty(t, result)
			tt.validate(t, result)
		})
	}
}

// TestCreateImageRequestValid tests validation of CreateImageRequest
func TestCreateImageRequestValid(t *testing.T) {
	tests := []struct {
		name      string
		request   CreateImageRequest
		wantValid bool
		wantMsg   string
	}{
		{
			name: "valid request",
			request: CreateImageRequest{
				Registry:    "harbor.example.com",
				ImageTag:    "myapp:v1.0",
				Description: "My application",
				IsShare:     true,
			},
			wantValid: true,
			wantMsg:   "",
		},
		{
			name: "valid request without optional fields",
			request: CreateImageRequest{
				ImageTag: "nginx:latest",
			},
			wantValid: true,
			wantMsg:   "",
		},
		{
			name: "invalid - empty image tag",
			request: CreateImageRequest{
				Registry:    "harbor.example.com",
				Description: "Test",
			},
			wantValid: false,
			wantMsg:   "imageTag is required",
		},
		{
			name:      "invalid - completely empty",
			request:   CreateImageRequest{},
			wantValid: false,
			wantMsg:   "imageTag is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, msg := tt.request.Valid()
			assert.Equal(t, tt.wantValid, valid)
			if !tt.wantValid {
				assert.Equal(t, tt.wantMsg, msg)
			}
		})
	}
}

// TestCreateRegistryRequestValidate tests validation of CreateRegistryRequest
func TestCreateRegistryRequestValidate(t *testing.T) {
	tests := []struct {
		name       string
		request    CreateRegistryRequest
		isCreate   bool
		wantErr    bool
		errMessage string
	}{
		{
			name: "valid create request",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				UserName: "admin",
				Password: "password123",
				Default:  true,
			},
			isCreate: true,
			wantErr:  false,
		},
		{
			name: "valid update request without password",
			request: CreateRegistryRequest{
				Id:       1,
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				UserName: "admin",
				Default:  false,
			},
			isCreate: false,
			wantErr:  false,
		},
		{
			name: "invalid - missing name",
			request: CreateRegistryRequest{
				Url:      "https://harbor.example.com",
				UserName: "admin",
				Password: "password",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "name is required",
		},
		{
			name: "invalid - missing url",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				UserName: "admin",
				Password: "password",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "url is required",
		},
		{
			name: "invalid - missing username",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				Password: "password",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "token is required",
		},
		{
			name: "invalid - missing password on create",
			request: CreateRegistryRequest{
				Name:     "MyRegistry",
				Url:      "https://harbor.example.com",
				UserName: "admin",
			},
			isCreate:   true,
			wantErr:    true,
			errMessage: "password is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate(tt.isCreate)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestBuildAuthVolumes tests building volumes and volume mounts for auth config
func TestBuildAuthVolumes(t *testing.T) {
	tests := []struct {
		name              string
		authConfigMapName string
		wantVolumeType    string // "Secret" or "ConfigMap"
		wantVolumeName    string
		wantMountPath     string
	}{
		{
			name:              "empty ConfigMap name - should use system Secret",
			authConfigMapName: "",
			wantVolumeType:    "Secret",
			wantVolumeName:    "registry-auth",
			wantMountPath:     "/root/.docker",
		},
		{
			name:              "with ConfigMap name - should use ConfigMap",
			authConfigMapName: "imptimg-123-abc-auth",
			wantVolumeType:    "ConfigMap",
			wantVolumeName:    "registry-auth",
			wantMountPath:     "/root/.docker",
		},
		{
			name:              "different ConfigMap name",
			authConfigMapName: "custom-auth-config",
			wantVolumeType:    "ConfigMap",
			wantVolumeName:    "registry-auth",
			wantMountPath:     "/root/.docker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			volumes, volumeMounts := buildAuthVolumes(tt.authConfigMapName)

			// Verify volumes
			assert.Len(t, volumes, 1, "should have exactly one volume")
			assert.Equal(t, tt.wantVolumeName, volumes[0].Name)

			if tt.wantVolumeType == "Secret" {
				assert.NotNil(t, volumes[0].VolumeSource.Secret, "volume should be Secret type")
				assert.Nil(t, volumes[0].VolumeSource.ConfigMap, "volume should not be ConfigMap type")
			} else {
				assert.NotNil(t, volumes[0].VolumeSource.ConfigMap, "volume should be ConfigMap type")
				assert.Nil(t, volumes[0].VolumeSource.Secret, "volume should not be Secret type")
				assert.Equal(t, tt.authConfigMapName, volumes[0].VolumeSource.ConfigMap.Name)
			}

			// Verify volume mounts
			assert.Len(t, volumeMounts, 1, "should have exactly one volume mount")
			assert.Equal(t, tt.wantVolumeName, volumeMounts[0].Name)
			assert.Equal(t, tt.wantMountPath, volumeMounts[0].MountPath)
			assert.True(t, volumeMounts[0].ReadOnly, "volume mount should be read-only")
		})
	}
}

// --- merged from image_auth_test.go ---

// stubHTTPClient is a minimal httpclient.Interface used to drive token fetching.
type stubHTTPClient struct {
	result *httpclient.Result
	err    error
}

func (s *stubHTTPClient) Get(string, ...string) (*httpclient.Result, error) { return s.result, s.err }
func (s *stubHTTPClient) Post(string, interface{}, ...string) (*httpclient.Result, error) {
	return s.result, s.err
}
func (s *stubHTTPClient) Put(string, interface{}, ...string) (*httpclient.Result, error) {
	return s.result, s.err
}
func (s *stubHTTPClient) Delete(string, ...string) (*httpclient.Result, error) {
	return s.result, s.err
}
func (s *stubHTTPClient) Do(*http.Request) (*httpclient.Result, error) { return s.result, s.err }
func (s *stubHTTPClient) GetBaseClient() *http.Client                  { return nil }

// TestDecryptRegistryAuthEmpty verifies empty credentials yield empty auth.
func TestDecryptRegistryAuthEmpty(t *testing.T) {
	h := &ImageHandler{}
	auth, err := h.decryptRegistryAuth(&model.RegistryInfo{})
	require.NoError(t, err)
	testifyassert.Empty(t, auth.Username)
	testifyassert.Empty(t, auth.Password)
}

// TestFetchDockerTokenSuccess verifies a token is parsed from the auth response.
func TestFetchDockerTokenSuccess(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusOK, Body: []byte(`{"token":"abc123"}`)},
	}}
	token, err := h.fetchDockerToken(context.Background(), "library/alpine")
	require.NoError(t, err)
	assert.Equal(t, "abc123", token)
}

// TestFetchDockerTokenNon200 verifies a non-200 response yields an error.
func TestFetchDockerTokenNon200(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusUnauthorized},
	}}
	_, err := h.fetchDockerToken(context.Background(), "library/alpine")
	testifyassert.Error(t, err)
}

// TestFetchDockerTokenBadJSON verifies an unparseable body yields an error.
func TestFetchDockerTokenBadJSON(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusOK, Body: []byte("not-json")},
	}}
	_, err := h.fetchDockerToken(context.Background(), "library/alpine")
	testifyassert.Error(t, err)
}

// TestGetDockerHubSystemCtx verifies a system context is built with the fetched token.
func TestGetDockerHubSystemCtx(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusOK, Body: []byte(`{"token":"tok"}`)},
	}}
	ctx, err := h.getDockerHubSystemCtx(context.Background(), "docker.io/library/alpine:latest")
	require.NoError(t, err)
	testifyassert.NotNil(t, ctx)
}

// --- merged from image_convert_test.go ---

// TestConvertOpsJobToExportedImageList verifies inputs/outputs/conditions parsing.
func TestConvertOpsJobToExportedImageList(t *testing.T) {
	jobs := []*dbclient.OpsJob{
		{
			JobId:      "job-1",
			Phase:      sql.NullString{String: "Succeeded", Valid: true},
			Inputs:     []byte("{workload:wl-1,label:custom}"),
			Outputs:    sql.NullString{String: `[{"name":"target","value":"harbor.io/p/app:tag"}]`, Valid: true},
			Conditions: sql.NullString{String: `[{"type":"Ready","status":"True","message":"done"}]`, Valid: true},
		},
	}
	result := convertOpsJobToExportedImageList(jobs)
	require.Len(t, result, 1)
	assert.Equal(t, "job-1", result[0].JobId)
	assert.Equal(t, "wl-1", result[0].Workload)
	assert.Equal(t, "custom", result[0].Label)
	assert.Equal(t, "harbor.io/p/app:tag", result[0].ImageName)
	assert.Equal(t, "done", result[0].Log)
}

// TestConvertOpsJobToExportedImageListEmpty verifies empty/zero-field jobs are handled.
func TestConvertOpsJobToExportedImageList_Empty(t *testing.T) {
	result := convertOpsJobToExportedImageList(nil)
	testifyassert.Len(t, result, 0)

	result = convertOpsJobToExportedImageList([]*dbclient.OpsJob{{JobId: "j"}})
	require.Len(t, result, 1)
	assert.Equal(t, "j", result[0].JobId)
}

// --- merged from image_delete_cleanup_test.go ---

// imageCleanupHandler builds an ImageHandler whose fake k8s client is seeded
// with an admin user + wildcard role (so authorization passes) plus any extra
// objects (e.g. the import Job under test).
func imageCleanupHandler(t *testing.T, mockDB *mock_client.MockInterface, objs ...ctrlclient.Object) (*ImageHandler, ctrlclient.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add amd scheme: %v", err)
	}
	if err := batchv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add batch scheme: %v", err)
	}
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"admin"}},
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "admin"},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"*"},
			GrantedUsers: []string{"*"},
			Verbs:        []v1.RoleVerb{"*"},
		}},
	}
	all := append([]ctrlclient.Object{admin, role}, objs...)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).Build()
	h := &ImageHandler{
		Client:           cl,
		dbClient:         mockDB,
		accessController: &authority.AccessController{Client: cl},
	}
	return h, cl
}

// TestDeleteImagePassesCurrentUser verifies S9: the delete records the current
// request user, not the stale DeletedBy value from the fetched row.
func TestDeleteImagePassesCurrentUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(&model.Image{ID: 7, DeletedBy: "olduser"}, nil)
	// Must be called with the current user "u1", not the stale "olduser".
	m.EXPECT().DeleteImage(gomock.Any(), int32(7), "u1").Return(nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(7)).Return(nil, nil)

	h, _ := imageCleanupHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	_, err := h.deleteImage(c)
	testifyassert.NoError(t, err)
}

// TestDeleteImageCleansUpImportJob verifies P3: deleting an imported image also
// deletes its import Kubernetes Job (best-effort). Tag is empty here so the
// Harbor artifact deletion is skipped and the test focuses on Job cleanup.
func TestDeleteImageCleansUpImportJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(&model.Image{ID: 7}, nil)
	m.EXPECT().DeleteImage(gomock.Any(), int32(7), "u1").Return(nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(7)).
		Return(&model.ImageImportJob{ID: 9, ImageID: 7, JobName: "imptimg-7-abc"}, nil)
	m.EXPECT().DeleteImageImportJob(gomock.Any(), int32(9)).Return(nil)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "imptimg-7-abc", Namespace: common.PrimusSafeNamespace},
	}
	h, cl := imageCleanupHandler(t, m, job)

	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	if _, err := h.deleteImage(c); err != nil {
		t.Fatalf("deleteImage returned error: %v", err)
	}

	got := &batchv1.Job{}
	err := cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "imptimg-7-abc", Namespace: common.PrimusSafeNamespace}, got)
	if err == nil {
		t.Fatal("expected import job to be deleted, but it still exists")
	}
}

// --- merged from image_export_test.go ---

// TestListExportedImage verifies export jobs are listed and converted.
func TestListExportedImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.OpsJob{{JobId: "j1", Phase: sql.NullString{String: "Succeeded", Valid: true}}}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(1, nil)

	h := importJobHandler(t, m)
	res, err := h.listExportedImage(ginCtx(t, http.MethodGet, "", nil))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// TestListPrewarmImage verifies prewarm jobs are listed.
func TestListPrewarmImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.OpsJob{{JobId: "p1"}}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(1, nil)

	h := importJobHandler(t, m)
	res, err := h.listPrewarmImage(ginCtx(t, http.MethodGet, "", nil))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// TestDeleteExportedImageEmptyID verifies the empty-id branch.
func TestDeleteExportedImageEmptyID(t *testing.T) {
	h := importJobHandler(t, mock_client.NewMockInterface(gomock.NewController(t)))
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", nil))
	testifyassert.Error(t, err)
}

// TestDeleteExportedImageNotFound verifies a missing job yields not-found.
func TestDeleteExportedImageNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "j1").Return(nil, nil)

	h := importJobHandler(t, m)
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "j1"}}))
	testifyassert.Error(t, err)
}

// TestDeleteExportedImageAlreadyDeleted verifies the already-deleted branch.
func TestDeleteExportedImageAlreadyDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "j1").Return(&dbclient.OpsJob{JobId: "j1", IsDeleted: true}, nil)

	h := importJobHandler(t, m)
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "j1"}}))
	testifyassert.Error(t, err)
}

// TestDeleteExportedImageSuccess verifies soft delete succeeds (no image name -> no harbor call).
func TestDeleteExportedImageSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "j1").Return(&dbclient.OpsJob{JobId: "j1"}, nil)
	m.EXPECT().SetOpsJobDeleted(gomock.Any(), "j1").Return(nil)

	h := importJobHandler(t, m)
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "j1"}}))
	testifyassert.NoError(t, err)
}

// --- merged from image_import_info_test.go ---

// TestGetImportImageInfoNoDefaultRegistry verifies missing default registry yields bad request.
func TestGetImportImageInfoNoDefaultRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(nil, nil)

	h := &ImageHandler{dbClient: m}
	_, err := h.getImportImageInfo(context.Background(),
		&ImportImageServiceRequest{Source: "docker.io/library/alpine:latest"}, nil)
	testifyassert.Error(t, err)
}

// TestGetImportImageInfoDBError verifies a registry lookup error is surfaced.
func TestGetImportImageInfoDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(nil, errors.New("db down"))

	h := &ImageHandler{dbClient: m}
	_, err := h.getImportImageInfo(context.Background(),
		&ImportImageServiceRequest{Source: "docker.io/library/alpine:latest"}, nil)
	testifyassert.Error(t, err)
}

// TestGetImportImageInfoBadSource verifies an invalid source image name is rejected.
func TestGetImportImageInfoBadSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(&model.RegistryInfo{URL: "harbor.io"}, nil)

	h := &ImageHandler{dbClient: m}
	_, err := h.getImportImageInfo(context.Background(),
		&ImportImageServiceRequest{Source: "noslash"}, nil)
	testifyassert.Error(t, err)
}

// TestImportImageBadBody verifies invalid JSON is rejected at the entry point.
func TestImportImageBadBody(t *testing.T) {
	h := importJobHandler(t, mock_client.NewMockInterface(gomock.NewController(t)))
	_, err := h.importImage(ginCtx(t, http.MethodPost, "{bad", nil))
	testifyassert.Error(t, err)
}

// TestImportImageNoDefaultRegistry drives the entry point through authorization into
// getImportImageInfo, which fails fast when no default push registry is configured.
func TestImportImageNoDefaultRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(nil, nil)

	h := importJobHandler(t, m)
	_, err := h.importImage(ginCtx(t, http.MethodPost,
		`{"source":"docker.io/library/alpine:latest"}`, nil))
	testifyassert.Error(t, err)
}

// --- merged from image_import_more_test.go ---

// importJobHandler builds a handler with admin authorization, a fake ctrl client
// (v1+corev1+batchv1) and a fake clientSet, plus the supplied db mock.
func importJobHandler(t *testing.T, m *mock_client.MockInterface) *ImageHandler {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "u1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{"admin"}},
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "admin"},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"*"},
			GrantedUsers: []string{"*"},
			Verbs:        []v1.RoleVerb{"*"},
		}},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(admin, role).Build()
	return &ImageHandler{
		Client:           cl,
		dbClient:         m,
		clientSet:        k8sfake.NewSimpleClientset(),
		accessController: &authority.AccessController{Client: cl},
	}
}

// TestNewImportImageJobPlatformSpecific verifies a platform-specific job spec is built.
func TestNewImportImageJobPlatformSpecific(t *testing.T) {
	job, err := newImportImageJob(1, "job-1", "syncer:latest", []string{"ps1"}, &ImportImageEnv{
		SourceImageName: "docker.io/library/alpine:latest",
		DestImageName:   "harbor.io/p/alpine:latest",
		OsArch:          "linux/amd64",
		Os:              "linux",
		Arch:            "amd64",
	}, "u1", "")
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, "job-1", job.Name)
	require.Len(t, job.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "syncer:latest", job.Spec.Template.Spec.Containers[0].Image)
}

// TestNewImportImageJobAllPlatforms verifies the all-platform branch and ConfigMap volume.
func TestNewImportImageJobAllPlatforms(t *testing.T) {
	job, err := newImportImageJob(2, "job-2", "syncer:latest", nil, &ImportImageEnv{
		SourceImageName: "src:tag",
		DestImageName:   "dst:tag",
		OsArch:          OsArchAll,
	}, "u1", "auth-cm")
	require.NoError(t, err)
	require.NotNil(t, job)
	// ConfigMap-backed auth volume should be present.
	require.NotEmpty(t, job.Spec.Template.Spec.Volumes)
	testifyassert.NotNil(t, job.Spec.Template.Spec.Volumes[0].ConfigMap)
}

// TestDispatchImportImageJob verifies a k8s Job is created (no user secret path).
func TestDispatchImportImageJob(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	h := importJobHandler(t, m)

	c := ginCtx(t, http.MethodPost, "", nil)
	job, err := h.dispatchImportImageJob(c, &model.Image{ID: 1, CreatedBy: "u1"},
		&model.ImageImportJob{SrcTag: "src:tag", DstName: "dst:tag", Os: "linux", Arch: "amd64"}, nil)
	require.NoError(t, err)
	require.NotNil(t, job)

	created := &batchv1.Job{}
	require.NoError(t, h.Client.Get(context.Background(),
		ctrlclient.ObjectKey{Namespace: job.Namespace, Name: job.Name}, created))
}

// TestRetryDispatchImportImageJobNotFound verifies a missing image yields not-found.
func TestRetryDispatchImportImageJobNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(5)).Return(nil, nil)

	h := importJobHandler(t, m)
	c := ginCtx(t, http.MethodPut, "", gin.Params{{Key: "id", Value: "5"}})
	_, err := h.retryDispatchImportImageJob(c)
	testifyassert.Error(t, err)
}

// TestRetryDispatchImportImageJobSuccess verifies the full retry path dispatches a job.
func TestRetryDispatchImportImageJobSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(5)).Return(&model.Image{ID: 5, CreatedBy: "u1"}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(5)).
		Return(&model.ImageImportJob{ID: 9, ImageID: 5, SrcTag: "src:tag", DstName: "dst:tag", Os: "linux", Arch: "amd64"}, nil)
	m.EXPECT().UpsertImage(gomock.Any(), gomock.Any()).Return(nil)
	m.EXPECT().UpsertImageImportJob(gomock.Any(), gomock.Any()).Return(nil)

	h := importJobHandler(t, m)
	c := ginCtx(t, http.MethodPut, "", gin.Params{{Key: "id", Value: "5"}})
	_, err := h.retryDispatchImportImageJob(c)
	testifyassert.NoError(t, err)
}

// TestUpsertImageRegistryInfoUpdate verifies the update branch (existing record) is exercised.
func TestUpsertImageRegistryInfoUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetRegistryInfoById(gomock.Any(), int32(3)).Return(&model.RegistryInfo{ID: 3}, nil)
	m.EXPECT().UpsertRegistryInfo(gomock.Any(), gomock.Any()).Return(nil)

	h := &ImageHandler{dbClient: m}
	// Empty username/password avoids crypto-dependent encryption.
	res, err := h.upsertImageRegistryInfo(context.Background(), &CreateRegistryRequest{
		Id: 3, Name: "r1", Url: "harbor.io",
	})
	require.NoError(t, err)
	assert.Equal(t, int32(3), res.ID)
}

// TestUpsertImageRegistryInfoCreate verifies the create branch (no existing record).
func TestUpsertImageRegistryInfoCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().UpsertRegistryInfo(gomock.Any(), gomock.Any()).Return(nil)

	h := &ImageHandler{dbClient: m}
	res, err := h.upsertImageRegistryInfo(context.Background(), &CreateRegistryRequest{
		Name: "r1", Url: "harbor.io",
	})
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// --- merged from image_importdetail_test.go ---

func TestGetImportingDetailHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(&model.Image{ID: 1}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).
		Return(&model.ImageImportJob{ID: 1, ImageID: 1}, nil)

	h := registryTestHandler(t, m)
	res, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetImportingDetailImageNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(nil, nil)

	h := registryTestHandler(t, m)
	_, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	testifyassert.Error(t, err)
}

func TestGetImportingDetailImportNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(&model.Image{ID: 1}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).Return(nil, nil)

	h := registryTestHandler(t, m)
	_, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	testifyassert.Error(t, err)
}

func TestGetImportingDetailBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	_, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "x"}}))
	testifyassert.Error(t, err)
}

func TestGetImportingLogsDBFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(&model.Image{ID: 1}, nil)
	// JobName empty -> returns DB log without touching OpenSearch.
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).
		Return(&model.ImageImportJob{ID: 1, ImageID: 1, Log: "log line", Os: "linux", Arch: "amd64"}, nil)

	h := registryTestHandler(t, m)
	res, err := h.getImportingLogs(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestGetImportingLogsBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	_, err := h.getImportingLogs(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "x"}}))
	testifyassert.Error(t, err)
}

// --- merged from image_k8s_test.go ---

func coreScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, v1.AddToScheme(s))
	return s
}

// TestGetAndValidateImageSecretSuccess verifies an image-type secret is returned.
func TestGetAndValidateImageSecretSuccess(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: string(v1.SecretImage)},
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(secret).Build()
	h := &ImageHandler{Client: cl}

	got, err := h.getAndValidateImageSecret(context.Background(), "sec1")
	require.NoError(t, err)
	assert.Equal(t, "sec1", got.Name)
}

// TestGetAndValidateImageSecretNotFound verifies a missing secret yields an error.
func TestGetAndValidateImageSecretNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	_, err := h.getAndValidateImageSecret(context.Background(), "missing")
	testifyassert.Error(t, err)
}

// TestGetAndValidateImageSecretWrongType verifies a non-image secret yields an error.
func TestGetAndValidateImageSecretWrongType(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec2",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: "other"},
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(secret).Build()
	h := &ImageHandler{Client: cl}
	_, err := h.getAndValidateImageSecret(context.Background(), "sec2")
	testifyassert.Error(t, err)
}

// TestCreateMergedAuthConfigMap verifies system and user auths are merged into a ConfigMap.
func TestCreateMergedAuthConfigMap(t *testing.T) {
	systemSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ImageImportSecretName,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"auths":{"sys.io":{"auth":"a"}}}`),
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(systemSecret).Build()
	h := &ImageHandler{Client: cl}

	userSecret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"user.io":{"auth":"b"}}}`),
		},
	}
	cm, err := h.createMergedAuthConfigMap(context.Background(), "job1", userSecret)
	require.NoError(t, err)
	testifyassert.Contains(t, cm.Data["config.json"], "sys.io")
	testifyassert.Contains(t, cm.Data["config.json"], "user.io")

	created := &corev1.ConfigMap{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{
		Name: "job1-auth", Namespace: common.PrimusSafeNamespace,
	}, created))
}

// TestCreateMergedAuthConfigMapMissingSystemSecret verifies an error when system secret is absent.
func TestCreateMergedAuthConfigMapMissingSystemSecret(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	_, err := h.createMergedAuthConfigMap(context.Background(), "job1", &corev1.Secret{})
	testifyassert.Error(t, err)
}

// --- merged from image_list_test.go ---

func TestCreateImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().UpsertImage(gomock.Any(), gomock.Any()).Return(nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodPost, `{"imageTag":"nginx:latest","description":"d"}`, nil)
	_, err := h.createImage(c)
	testifyassert.NoError(t, err)
}

func TestCreateImageHandlerInvalid(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodPost, `{"description":"no tag"}`, nil)
	_, err := h.createImage(c)
	testifyassert.Error(t, err)
}

func TestDeleteImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(&model.Image{ID: 7}, nil)
	m.EXPECT().DeleteImage(gomock.Any(), int32(7), gomock.Any()).Return(nil)
	// Best-effort import-job cleanup looks up the import job after deletion.
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(7)).Return(nil, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	_, err := h.deleteImage(c)
	testifyassert.NoError(t, err)
}

func TestDeleteImageHandlerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(nil, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	_, err := h.deleteImage(c)
	testifyassert.NoError(t, err)
}

func TestDeleteImageHandlerBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "x"}})
	_, err := h.deleteImage(c)
	testifyassert.Error(t, err)
}

func TestListImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectImages(gomock.Any(), gomock.Any()).Return(
		[]*model.Image{{ID: 1, Tag: "harbor.io/proj/app:v1", CreatedAt: time.Now()}}, 1, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listImage(c)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestListExportedImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbClient.OpsJob{}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(0, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listExportedImage(c)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestListPrewarmImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbClient.OpsJob{}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(0, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listPrewarmImage(c)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, res)
}

func TestDeleteExportedImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// Job has no outputs -> no harbor deletion attempted.
	m.EXPECT().GetOpsJob(gomock.Any(), "job-1").Return(&dbClient.OpsJob{JobId: "job-1"}, nil)
	m.EXPECT().SetOpsJobDeleted(gomock.Any(), "job-1").Return(nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "job-1"}})
	_, err := h.deleteExportedImage(c)
	testifyassert.NoError(t, err)
}

func TestDeleteExportedImageHandlerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "job-1").Return(nil, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "job-1"}})
	_, err := h.deleteExportedImage(c)
	testifyassert.Error(t, err)
}

// --- merged from image_logs_test.go ---

// TestGetImportingDetailSuccess verifies layer details are returned.
func TestGetImportingDetailSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(3)).Return(&model.Image{ID: 3}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(3)).Return(&model.ImageImportJob{ID: 9, ImageID: 3}, nil)

	h := importJobHandler(t, m)
	res, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "3"}}))
	require.NoError(t, err)
	testifyassert.NotNil(t, res)
}

// TestGetImportingLogsImportNotFound verifies missing import record yields not-found.
func TestGetImportingLogsImportNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(3)).Return(&model.Image{ID: 3}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(3)).Return(nil, nil)

	h := importJobHandler(t, m)
	_, err := h.getImportingLogs(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "3"}}))
	testifyassert.Error(t, err)
}

// TestConvertOpsJobToPrewarmImageList verifies prewarm job conversion across all branches.
func TestConvertOpsJobToPrewarmImageList(t *testing.T) {
	m := mock_client.NewMockInterface(gomock.NewController(t))
	h := importJobHandler(t, m)

	jobs := []*dbclient.OpsJob{
		{
			JobId:      "p1",
			Phase:      sql.NullString{String: "Running", Valid: true},
			Inputs:     []byte("{image:img:tag,workspace:ws-1}"),
			Outputs:    sql.NullString{String: `[{"name":"status","value":"InProgress"},{"name":"prewarm_progress","value":"50"},{"name":"nodes_ready","value":"1"},{"name":"nodes_total","value":"2"}]`, Valid: true},
			Conditions: sql.NullString{String: `[{"type":"X","status":"True","message":"warn"}]`, Valid: true},
		},
	}
	result := h.convertOpsJobToPrewarmImageList(context.Background(), jobs)
	require.Len(t, result, 1)
	assert.Equal(t, "img:tag", result[0].ImageName)
	assert.Equal(t, "ws-1", result[0].WorkspaceId)
	assert.Equal(t, "InProgress", result[0].Status)
	assert.Equal(t, "50", result[0].PrewarmProgress)
	assert.Equal(t, "warn", result[0].ErrorMessage)
}

// --- merged from image_manifest_test.go ---

// TestCalculateManifestSizeOCI verifies layer+config sizes are summed for OCI manifests.
func TestCalculateManifestSizeOCI(t *testing.T) {
	h := &ImageHandler{}
	m := imagespecv1.Manifest{
		Config: imagespecv1.Descriptor{Size: 50},
		Layers: []imagespecv1.Descriptor{{Size: 100}, {Size: 200}},
	}
	raw, err := json.Marshal(m)
	testifyassert.NoError(t, err)

	size, err := h.calculateManifestSize(raw, imagespecv1.MediaTypeImageManifest)
	testifyassert.NoError(t, err)
	assert.Equal(t, int64(350), size)
}

// TestCalculateManifestSizeUnsupported verifies unsupported manifest types error out.
func TestCalculateManifestSizeUnsupported(t *testing.T) {
	h := &ImageHandler{}
	_, err := h.calculateManifestSize([]byte("{}"), "application/unknown")
	testifyassert.Error(t, err)
}

// TestCalculateManifestSizeBadJSON verifies parsing errors are reported.
func TestCalculateManifestSizeBadJSON(t *testing.T) {
	h := &ImageHandler{}
	_, err := h.calculateManifestSize([]byte("not-json"), imagespecv1.MediaTypeImageManifest)
	testifyassert.Error(t, err)
}

// TestExtractPlatformDigestOCIIndex verifies the matching platform digest is returned.
func TestExtractPlatformDigestOCIIndex(t *testing.T) {
	h := &ImageHandler{}
	index := imagespecv1.Index{
		Manifests: []imagespecv1.Descriptor{
			{
				Digest:   "sha256:abc",
				Platform: &imagespecv1.Platform{OS: "linux", Architecture: "amd64"},
			},
		},
	}
	raw, err := json.Marshal(index)
	testifyassert.NoError(t, err)

	digest, err := h.extractPlatformDigest(raw, imagespecv1.MediaTypeImageIndex, "linux", "amd64")
	testifyassert.NoError(t, err)
	assert.Equal(t, "sha256:abc", digest.String())
}

// TestExtractPlatformDigestNoMatch verifies an error when no platform matches.
func TestExtractPlatformDigestNoMatch(t *testing.T) {
	h := &ImageHandler{}
	index := imagespecv1.Index{
		Manifests: []imagespecv1.Descriptor{
			{
				Digest:   "sha256:abc",
				Platform: &imagespecv1.Platform{OS: "linux", Architecture: "arm64"},
			},
		},
	}
	raw, err := json.Marshal(index)
	testifyassert.NoError(t, err)

	_, err = h.extractPlatformDigest(raw, imagespecv1.MediaTypeImageIndex, "linux", "amd64")
	testifyassert.Error(t, err)
}

// TestExtractPlatformDigestBadIndex verifies parse errors are reported.
func TestExtractPlatformDigestBadIndex(t *testing.T) {
	h := &ImageHandler{}
	_, err := h.extractPlatformDigest([]byte("bad"), imagespecv1.MediaTypeImageIndex, "linux", "amd64")
	testifyassert.Error(t, err)
}

// --- merged from image_more_test.go ---

func TestGetDesiredImagePullSecret(t *testing.T) {
	h := &ImageHandler{}
	// Registries without username are skipped; result is a valid empty-auth secret.
	secret, err := h.getDesiredImagePullSecret([]*model.RegistryInfo{
		{Name: "r1", URL: "harbor.io", Username: ""},
	})
	testifyassert.NoError(t, err)
	assert.Equal(t, ImagePullSecretName, secret.Name)
	assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
	testifyassert.Contains(t, secret.StringData, ".dockerconfigjson")
}

func TestGetDesiredImageImportSecret(t *testing.T) {
	h := &ImageHandler{}
	secret, err := h.getDesiredImageImportSecret([]*model.RegistryInfo{
		{Name: "r1", URL: "harbor.io", Username: ""},
	})
	testifyassert.NoError(t, err)
	assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
	testifyassert.Contains(t, secret.StringData, "config.json")
}

func TestRefreshImagePullSecretsCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return([]*model.RegistryInfo{}, nil)

	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset()}
	testifyassert.NoError(t, h.refreshImagePullSecrets(context.Background()))
}

func TestRefreshImageImportSecretsCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return([]*model.RegistryInfo{}, nil)

	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset()}
	testifyassert.NoError(t, h.refreshImageImportSecrets(context.Background()))
}

func TestRefreshImagePullSecretsDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return(nil, errors.New("db error"))

	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset()}
	testifyassert.Error(t, h.refreshImagePullSecrets(context.Background()))
}

func TestRefreshImagePullSecretsUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return([]*model.RegistryInfo{}, nil)

	// Pre-create the secret so the update path is exercised.
	existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      ImagePullSecretName,
		Namespace: "primus-safe",
	}}
	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset(existing)}
	testifyassert.NoError(t, h.refreshImagePullSecrets(context.Background()))
}

func TestCvtCreateRegistryRequestToRegistryInfoEmptyCreds(t *testing.T) {
	h := &ImageHandler{}
	info, err := h.cvtCreateRegistryRequestToRegistryInfo(&CreateRegistryRequest{
		Name: "reg", Url: "https://harbor.io", Default: true,
	})
	testifyassert.NoError(t, err)
	assert.Equal(t, "reg", info.Name)
	assert.Equal(t, "https://harbor.io", info.URL)
	testifyassert.Empty(t, info.Password)
	testifyassert.Empty(t, info.Username)
	testifyassert.True(t, info.Default)
}

func TestCvtDBRegistryInfoToViewNoUsername(t *testing.T) {
	h := &ImageHandler{}
	now := time.Now()
	view, err := h.cvtDBRegistryInfoToView(&model.RegistryInfo{
		ID: 1, Name: "reg", URL: "https://harbor.io", Default: true,
		CreatedAt: now, UpdatedAt: now,
	})
	testifyassert.NoError(t, err)
	assert.Equal(t, int32(1), view.Id)
	assert.Equal(t, "reg", view.Name)
	testifyassert.Empty(t, view.Username)
}

func TestListImagePullSecretsName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	dockerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "docker-1", Namespace: "ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	opaque := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "opaque-1", Namespace: "ns"},
		Type:       corev1.SecretTypeOpaque,
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(dockerSecret, opaque).Build()

	h := &ImageHandler{}
	names, err := h.listImagePullSecretsName(context.Background(), cl, "ns")
	testifyassert.NoError(t, err)
	assert.Equal(t, []string{"docker-1"}, names)
}

// --- merged from image_pure_test.go ---

func TestParseImageTag(t *testing.T) {
	host, repo, tag, err := parseImageTag("harbor.example.com/project/app:v1.0")
	testifyassert.NoError(t, err)
	assert.Equal(t, "harbor.example.com", host)
	assert.Equal(t, "project/app", repo)
	assert.Equal(t, "v1.0", tag)

	_, _, _, err = parseImageTag("noslash")
	testifyassert.Error(t, err)

	_, _, _, err = parseImageTag("host/repo-without-tag")
	testifyassert.Error(t, err)
}

func TestParseImageID(t *testing.T) {
	id, err := parseImageID("123")
	testifyassert.NoError(t, err)
	assert.Equal(t, int32(123), id)

	_, err = parseImageID("")
	testifyassert.Error(t, err)

	_, err = parseImageID("abc")
	testifyassert.Error(t, err)
}

func TestDeserializeParams(t *testing.T) {
	testifyassert.Nil(t, deserializeParams(""))
	testifyassert.Nil(t, deserializeParams("{"))

	got := deserializeParams("{workload:wl-1,image:img-1}")
	testifyassert.Len(t, got, 2)
	assert.Equal(t, "workload", got[0].Name)
	assert.Equal(t, "wl-1", got[0].Value)
	assert.Equal(t, "image", got[1].Name)
	assert.Equal(t, "img-1", got[1].Value)

	// Quoted entries are trimmed.
	got = deserializeParams(`{"label:val"}`)
	testifyassert.Len(t, got, 1)
	assert.Equal(t, "label", got[0].Name)
	assert.Equal(t, "val", got[0].Value)
}

func TestExtractRegistryHost(t *testing.T) {
	assert.Equal(t, "docker.io", extractRegistryHost("nginx:latest"))
	assert.Equal(t, "docker.io", extractRegistryHost("rocm/image:tag"))
	assert.Equal(t, "ghcr.io", extractRegistryHost("ghcr.io/org/image:tag"))
	assert.Equal(t, "localhost:5000", extractRegistryHost("localhost:5000/img:tag"))
}

func TestParseHarborImageName(t *testing.T) {
	project, repo, ref, err := parseHarborImageName("harbor.example.com/Custom/rocm/7.0-preview:20250112")
	testifyassert.NoError(t, err)
	assert.Equal(t, "Custom", project)
	assert.Equal(t, "rocm/7.0-preview", repo)
	assert.Equal(t, "20250112", ref)

	_, _, _, err = parseHarborImageName("noslash")
	testifyassert.Error(t, err)

	_, _, _, err = parseHarborImageName("host/path-without-tag")
	testifyassert.Error(t, err)

	_, _, _, err = parseHarborImageName("host/onlyproject:tag")
	testifyassert.Error(t, err)
}

func TestTransEnvMapToEnv(t *testing.T) {
	envs := transEnvMapToEnv(map[string]string{"A": "1", "B": "2"})
	testifyassert.Len(t, envs, 2)
	m := map[string]string{}
	for _, e := range envs {
		m[e.Name] = e.Value
	}
	assert.Equal(t, "1", m["A"])
	assert.Equal(t, "2", m["B"])
}

func TestDefaultSyncImageEnv(t *testing.T) {
	env := defaultSyncImageEnv()
	assert.Equal(t, StringValueTrue, env[DEBUG])
	assert.Equal(t, "docker", env[SourceType])
	assert.Equal(t, "docker", env[DestinationType])
	assert.Equal(t, ApiServiceName, env[UpstreamDomain])
}

func TestDecodeJsonb(t *testing.T) {
	src := map[string]interface{}{"digest": "sha256:abc", "size": 100}
	var rd RelationDigest
	testifyassert.NoError(t, decodeJsonb(src, &rd))
	assert.Equal(t, "sha256:abc", rd.Digest)
	assert.Equal(t, int64(100), rd.Size)
}

func TestNewHTTPClientSkipTLS(t *testing.T) {
	c := newHTTPClientSkipTLS()
	testifyassert.NotNil(t, c)
	assert.Equal(t, 8*time.Second, c.Timeout)
}

func TestBuildImportLogSearchBody(t *testing.T) {
	body := buildImportLogSearchBody("job-1", time.Now().Add(-time.Hour), time.Now(), 100, "asc")
	testifyassert.NotEmpty(t, body)
	testifyassert.Contains(t, string(body), "job-1")
}

func TestBuildExportImageJobQuery(t *testing.T) {
	q := &ImageServiceRequest{}
	sqlizer, orderBy := buildExportImageJobQuery(q)
	testifyassert.NotNil(t, sqlizer)
	testifyassert.Len(t, orderBy, 1)

	q2 := &ImageServiceRequest{UserName: "u1", Ready: true, Workload: "wl-1", Order: "asc"}
	sqlizer2, orderBy2 := buildExportImageJobQuery(q2)
	testifyassert.NotNil(t, sqlizer2)
	testifyassert.Contains(t, orderBy2[0], "ASC")
}

func TestBuildPrewarmImageJobQuery(t *testing.T) {
	q := &ImageServiceRequest{}
	sqlizer, orderBy := buildPrewarmImageJobQuery(q)
	testifyassert.NotNil(t, sqlizer)
	testifyassert.Len(t, orderBy, 1)

	running := &ImageServiceRequest{Image: "img", Workspace: "ws", Status: "Running", UserName: "u", Ready: true}
	s2, _ := buildPrewarmImageJobQuery(running)
	testifyassert.NotNil(t, s2)

	other := &ImageServiceRequest{Status: "Succeeded"}
	s3, _ := buildPrewarmImageJobQuery(other)
	testifyassert.NotNil(t, s3)
}

func TestCvtImageToResponse(t *testing.T) {
	now := time.Now()
	images := []*model.Image{
		{ID: 1, Tag: "harbor.io/proj/app:v1", Description: "d1", CreatedBy: "u", CreatedAt: now},
		{ID: 2, Tag: "harbor.io/proj/app:v2", Description: "d2", CreatedBy: "u", CreatedAt: now},
		{ID: 3, Tag: "invalid-tag-no-slash", Description: "d3", CreatedBy: "u", CreatedAt: now},
	}
	res := cvtImageToResponse(images, DefaultOS, DefaultArch)
	// First two share repo -> grouped; third is fallback repo.
	testifyassert.GreaterOrEqual(t, len(res), 2)
	var appItem *GetImageResponseItem
	for i := range res {
		if res[i].Repo == "proj/app" {
			appItem = &res[i]
		}
	}
	testifyassert.NotNil(t, appItem)
	testifyassert.Len(t, appItem.Artifacts, 2)
}

// --- merged from image_secret_test.go ---

// TestBuildImagePullSecrets verifies secret names are wrapped into object references.
func TestBuildImagePullSecrets(t *testing.T) {
	refs := buildImagePullSecrets([]string{"a", "b"})
	testifyassert.Len(t, refs, 2)
	assert.Equal(t, "a", refs[0].Name)
	assert.Equal(t, "b", refs[1].Name)

	testifyassert.Len(t, buildImagePullSecrets(nil), 0)
}

// TestExtractAuthFromSecretDirect verifies username/password is read directly.
func TestExtractAuthFromSecretDirect(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"harbor.example.com":{"username":"u","password":"p"}}}`),
		},
	}
	auth, err := h.extractAuthFromSecret("harbor.example.com", secret)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, auth)
	assert.Equal(t, "u", auth.Username)
	assert.Equal(t, "p", auth.Password)
}

// TestExtractAuthFromSecretBase64 verifies credentials are decoded from the auth field.
func TestExtractAuthFromSecretBase64(t *testing.T) {
	h := &ImageHandler{}
	encoded := base64.StdEncoding.EncodeToString([]byte("user1:pass1"))
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"reg.io":{"auth":"` + encoded + `"}}}`),
		},
	}
	auth, err := h.extractAuthFromSecret("reg.io", secret)
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, auth)
	assert.Equal(t, "user1", auth.Username)
	assert.Equal(t, "pass1", auth.Password)
}

// TestExtractAuthFromSecretNoConfig verifies a missing dockerconfigjson returns nil.
func TestExtractAuthFromSecretNoConfig(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{Data: map[string][]byte{}}
	auth, err := h.extractAuthFromSecret("reg.io", secret)
	testifyassert.NoError(t, err)
	testifyassert.Nil(t, auth)
}

// TestExtractAuthFromSecretNoMatch verifies a non-matching host returns nil.
func TestExtractAuthFromSecretNoMatch(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"other.io":{"username":"u","password":"p"}}}`),
		},
	}
	auth, err := h.extractAuthFromSecret("reg.io", secret)
	testifyassert.NoError(t, err)
	testifyassert.Nil(t, auth)
}

// TestExtractAuthFromSecretBadJSON verifies invalid config json reports an error.
func TestExtractAuthFromSecretBadJSON(t *testing.T) {
	h := &ImageHandler{}
	secret := &corev1.Secret{
		Data: map[string][]byte{".dockerconfigjson": []byte("not-json")},
	}
	_, err := h.extractAuthFromSecret("reg.io", secret)
	testifyassert.Error(t, err)
}

// --- merged from image_sysctx_test.go ---

// TestExistImageValidFound verifies an existing image returns its id.
func TestExistImageValidFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").
		Return(&model.Image{ID: 7, Tag: "reg/app:tag"}, nil)

	h := &ImageHandler{dbClient: m}
	id, err := h.existImageValid(context.Background(), "reg/app:tag")
	require.NoError(t, err)
	assert.Equal(t, int32(7), id)
}

// TestExistImageValidNotFound verifies a nil image returns zero id.
func TestExistImageValidNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").Return(nil, nil)

	h := &ImageHandler{dbClient: m}
	id, err := h.existImageValid(context.Background(), "reg/app:tag")
	require.NoError(t, err)
	assert.Equal(t, int32(0), id)
}

// TestExistImageValidDBError verifies a db error is surfaced.
func TestExistImageValidDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))

	h := &ImageHandler{dbClient: m}
	_, err := h.existImageValid(context.Background(), "reg/app:tag")
	testifyassert.Error(t, err)
}

// TestGetImageSystemCtxUserSecret verifies a matching user secret short-circuits auth resolution.
func TestGetImageSystemCtxUserSecret(t *testing.T) {
	h := &ImageHandler{}
	userSecret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"harbor.example.com":{"username":"u","password":"p"}}}`),
		},
	}
	sysCtx, err := h.getImageSystemCtx(context.Background(), "harbor.example.com", "harbor.example.com/p/i:t", userSecret)
	require.NoError(t, err)
	require.NotNil(t, sysCtx)
	require.NotNil(t, sysCtx.DockerAuthConfig)
	assert.Equal(t, "u", sysCtx.DockerAuthConfig.Username)
}

// TestGetImageSystemCtxFromDB verifies registry info from the database is used when no user secret is given.
func TestGetImageSystemCtxFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// Empty credentials avoid crypto dependency (decrypt is skipped for empty fields).
	m.EXPECT().GetRegistryInfoByUrl(gomock.Any(), "reg.example.com").
		Return(&model.RegistryInfo{URL: "reg.example.com"}, nil)

	h := &ImageHandler{dbClient: m}
	sysCtx, err := h.getImageSystemCtx(context.Background(), "reg.example.com", "reg.example.com/p/i:t", nil)
	require.NoError(t, err)
	require.NotNil(t, sysCtx)
}
