/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/cespare/xxhash/v2"
	manifestv5 "github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/transports/alltransports"
	v5types "github.com/containers/image/v5/types"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	imagedigest "github.com/opencontainers/go-digest"
	imagespecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// createImage creates a new image record in the database.
// Validates the request and persists the image metadata with current timestamp and user info.
func (h *ImageHandler) createImage(c *gin.Context) (interface{}, error) {
	req := &CreateImageRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "fail to parse job request", "body", string(body))
		return nil, err
	}
	if valid, reason := req.Valid(); !valid {
		return nil, commonerrors.NewBadRequest(reason)
	}

	image := &model.Image{
		Tag:         req.ImageTag,
		Description: req.Description,
		CreatedAt:   time.Now(),
		CreatedBy:   c.GetString(common.UserId),
	}
	if err := h.dbClient.UpsertImage(c, image); err != nil {
		return nil, err
	}
	return nil, nil
}

// deleteImage soft-deletes an image by ID from the database.
// Returns nil if the image doesn't exist, otherwise marks it as deleted.
func (h *ImageHandler) deleteImage(c *gin.Context) (interface{}, error) {
	imageID, err := parseImageID(c.Param("id"))
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.ImageImportKind,
		Verb:         v1.DeleteVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	existImage, err := h.dbClient.GetImage(c, imageID)
	if err != nil {
		return nil, err
	}
	if existImage == nil {
		return nil, nil
	}

	if err := h.dbClient.DeleteImage(c, imageID, existImage.DeletedBy); err != nil {
		return nil, err
	}
	return nil, nil
}

// listImage retrieves a paginated list of images based on filter criteria.
// Supports both flat format (simple list) and grouped format (by repository).
func (h *ImageHandler) listImage(c *gin.Context) (interface{}, error) {
	query, err := parseListImageQuery(c)
	if err != nil {
		klog.ErrorS(err, "fail to parseListImageQuery")
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.ImageImportKind,
		Verb:         v1.ListVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	images, count, err := h.dbClient.SelectImages(c, &dbClient.ImageFilter{
		UserName: query.UserName,
		Tag:      query.Tag,
		OrderBy:  query.OrderBy,
		Order:    query.Order,
		PageNum:  query.PageNum,
		PageSize: query.PageSize,
		Ready:    query.Ready,
	})
	if err != nil {
		klog.ErrorS(err, "fail to SelectImages", "sql")
		return nil, err
	}

	if query.Flat {
		return cvtImageToFlatResponse(images), nil
	}

	return &GetImageResponse{
		TotalCount: count,
		Items:      cvtImageToResponse(images, DefaultOS, DefaultArch),
	}, nil
}

// cvtImageToFlatResponse converts database images to a flat list format.
// Each image contains basic metadata without grouping by repository.
func cvtImageToFlatResponse(images []*model.Image) []Image {
	res := make([]Image, 0, len(images))
	for _, image := range images {
		res = append(res, Image{
			Id:          image.ID,
			Tag:         image.Tag,
			Description: image.Description,
			CreatedBy:   image.CreatedBy,
			CreatedAt:   image.CreatedAt.Unix(),
			SecretId:    image.SecretID,
		})
	}
	return res
}

// listExportedImage lists images that were exported from workloads by querying ops_job table.
func (h *ImageHandler) listExportedImage(c *gin.Context) (interface{}, error) {
	query, err := parseListImageQuery(c)
	if err != nil {
		klog.ErrorS(err, "fail to parseListImageQuery")
		return nil, err
	}

	// Build SQL query for export jobs from ops_job table
	dbSql, orderBy := buildExportImageJobQuery(query)

	// Query export jobs from ops_job table
	jobs, err := h.dbClient.SelectJobs(c, dbSql, orderBy, query.PageSize, (query.PageNum-1)*query.PageSize)
	if err != nil {
		klog.ErrorS(err, "failed to query export jobs")
		return nil, err
	}

	// Count total records
	count, err := h.dbClient.CountJobs(c, dbSql)
	if err != nil {
		klog.ErrorS(err, "failed to count export jobs")
		return nil, err
	}

	// Convert ops_job records to simplified list format
	items := convertOpsJobToExportedImageList(jobs)

	results := &ExportedImageListResponse{
		TotalCount: count,
		Items:      items,
	}

	klog.V(4).Infof("listed %d exported images", len(items))
	return results, nil
}

// listPrewarmImage lists prewarm image jobs by querying ops_job table.
func (h *ImageHandler) listPrewarmImage(c *gin.Context) (interface{}, error) {
	query, err := parseListImageQuery(c)
	if err != nil {
		klog.ErrorS(err, "fail to parseListImageQuery")
		return nil, err
	}

	// Build SQL query for prewarm jobs from ops_job table
	dbSql, orderBy := buildPrewarmImageJobQuery(query)

	// Query prewarm jobs from ops_job table
	jobs, err := h.dbClient.SelectJobs(c, dbSql, orderBy, query.PageSize, (query.PageNum-1)*query.PageSize)
	if err != nil {
		klog.ErrorS(err, "failed to query prewarm jobs")
		return nil, err
	}

	// Count total records
	count, err := h.dbClient.CountJobs(c, dbSql)
	if err != nil {
		klog.ErrorS(err, "failed to count prewarm jobs")
		return nil, err
	}

	// Convert ops_job records to prewarm image list format
	items := h.convertOpsJobToPrewarmImageList(c.Request.Context(), jobs)

	results := &PrewarmImageListResponse{
		TotalCount: count,
		Items:      items,
	}

	klog.V(4).Infof("listed %d prewarm images", len(items))
	return results, nil
}

// parseListImageQuery extracts and validates query parameters for listing images.
// Sets default values for pagination, ordering, and filters if not provided.
func parseListImageQuery(c *gin.Context) (*ImageServiceRequest, error) {
	query := &ImageServiceRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.PageNum <= 0 {
		query.PageNum = 1
	}
	if query.PageSize <= 0 {
		query.PageSize = DefaultQueryLimit
	}
	if query.Order == "" {
		query.Order = dbClient.DESC
	}
	if query.OrderBy == "" {
		query.OrderBy = dbClient.CreatedTime
	} else {
		query.OrderBy = strings.ToLower(query.OrderBy)
	}
	if query.Flat {
		query.Ready = true
	}
	return query, nil
}

// getImportingDetail retrieves detailed layer information for an importing image.
// Returns the import job's layer details if the image and import job exist.
func (h *ImageHandler) getImportingDetail(c *gin.Context) (*ImportDetailResponse, error) {
	id, err := parseImageID(c.Param("id"))
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.ImageImportKind,
		Verb:         v1.GetVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	existImage, err := h.dbClient.GetImage(c, id)
	if err != nil {
		return nil, err
	}
	if existImage == nil {
		return nil, commonerrors.NewNotFound("get image by id", strconv.Itoa(int(id)))
	}

	importImage, err := h.dbClient.GetImportImageByImageID(c, existImage.ID)
	if err != nil {
		return nil, err
	}
	if importImage == nil {
		return nil, commonerrors.NewNotFound("get import image by id", strconv.Itoa(int(id)))
	}

	return &ImportDetailResponse{
		LayersDetail: importImage.Layer,
	}, nil
}

// cvtImageToResponse converts database images to a grouped response format.
// Groups images by registry host and repository, includes platform-specific metadata.
func cvtImageToResponse(images []*model.Image, os, arch string) []GetImageResponseItem {
	repoMap := make(map[string]int)
	res := make([]GetImageResponseItem, 0)

	for _, image := range images {
		registryHost, repo, tag, err := parseImageTag(image.Tag)
		if err != nil {
			klog.Errorf("image:%s is invalid, skip: %v", image.Tag, err)
			continue
		}

		artifact := ArtifactItem{
			ImageTag:    tag,
			Description: image.Description,
			CreatedTime: timeutil.FormatRFC3339(image.CreatedAt),
			UserName:    image.CreatedBy,
			Status:      image.Status,
			Id:          image.ID,
			IncludeType: image.Source,
			SecretId:    image.SecretID,
		}

		// Extract platform-specific digest and size
		if image.RelationDigest != nil {
			archKey := fmt.Sprintf(OSArchFormat, os, arch)
			relationDigestMap := make(map[string]*RelationDigest)
			if err := decodeJsonb(image.RelationDigest, &relationDigestMap); err == nil && relationDigestMap[archKey] != nil {
				artifact.Size = relationDigestMap[archKey].Size
				artifact.Arch = archKey
				artifact.Os = os
				artifact.Digest = relationDigestMap[archKey].Digest
			}
		}

		fullUrl := strings.Join([]string{registryHost, repo}, "/")
		if index, ok := repoMap[fullUrl]; !ok {
			// New repository - create new entry
			res = append(res, GetImageResponseItem{
				RegistryHost: registryHost,
				Repo:         repo,
				Artifacts:    []ArtifactItem{artifact},
			})
			repoMap[fullUrl] = len(res) - 1
		} else {
			// Existing repository - append artifact
			res[index].Artifacts = append(res[index].Artifacts, artifact)
		}
	}
	return res
}

// parseImageTag extracts registry host, repository, and tag from an image tag string.
// Expects format: <host>/<repo>:<tag> and returns an error if format is invalid.
func parseImageTag(imageTag string) (registryHost, repo, tag string, err error) {
	parts := strings.SplitN(imageTag, "/", 2)
	if len(parts) != 2 {
		return "", "", "", fmt.Errorf("invalid format: missing '/'")
	}
	registryHost = parts[0]

	repoTag := strings.SplitN(parts[1], ":", 2)
	if len(repoTag) != 2 {
		return "", "", "", fmt.Errorf("invalid format: missing ':'")
	}
	repo, tag = repoTag[0], repoTag[1]
	return registryHost, repo, tag, nil
}

// decodeJsonb decodes a JSONB map into a target struct.
// Performs a marshal-unmarshal cycle to convert between generic map and typed struct.
func decodeJsonb(data map[string]interface{}, to interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonBytes, to)
}

// importImage initiates an asynchronous image import job from source to destination registry.
// Creates database records, dispatches a Kubernetes job, and updates status to importing.
func (h *ImageHandler) importImage(c *gin.Context) (interface{}, error) {
	body := &ImportImageServiceRequest{}
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}

	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.ImageImportKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	// Get and validate user secret once if provided, reuse throughout the flow
	var userSecret *corev1.Secret
	if body.SecretId != "" {
		var err error
		userSecret, err = h.getAndValidateImageSecret(c.Request.Context(), body.SecretId)
		if err != nil {
			return nil, err
		}
	}

	userName := c.GetString(common.UserName)
	imageInfo, err := h.getImportImageInfo(c.Request.Context(), body, userSecret)
	if err != nil {
		return nil, err
	}

	// Check if image already exists
	existImageID, err := h.existImageValid(c, imageInfo.DestImageName)
	if err != nil {
		return nil, err
	}
	if existImageID != 0 {
		return &ImportImageResponse{
			AlreadyImageID: existImageID,
			Message:        "Image already existed. We don't need to import it again",
		}, nil
	}

	// Create image record
	relationDigest := map[string]interface{}{
		imageInfo.OsArch: &RelationDigest{Digest: "", Size: imageInfo.Size},
	}
	dbImage := &model.Image{
		Tag:            imageInfo.DestImageName,
		CreatedBy:      userName,
		CreatedAt:      time.Now().UTC(),
		Description:    fmt.Sprintf("Import from %s", imageInfo.SourceImageName),
		Status:         common.ImageImportPendingStatus,
		RelationDigest: relationDigest,
		Source:         "import",
		SecretID:       body.SecretId, // Associate secret with image for private image authentication
	}
	if err := h.dbClient.UpsertImage(c, dbImage); err != nil {
		return nil, err
	}

	// Create import job record
	importImageInfo := &model.ImageImportJob{
		SrcTag:    imageInfo.SourceImageName,
		DstName:   imageInfo.DestImageName,
		Os:        imageInfo.Os,
		Arch:      imageInfo.Arch,
		CreatedAt: time.Now().UTC(),
		ImageID:   dbImage.ID,
	}
	if err := h.dbClient.UpsertImageImportJob(c, importImageInfo); err != nil {
		return nil, err
	}

	// Dispatch Kubernetes job (pass userSecret to avoid re-fetching)
	job, err := h.dispatchImportImageJob(c, dbImage, importImageInfo, userSecret)
	if err != nil {
		return nil, err
	}

	// Update status to importing
	dbImage.Status = common.ImageImportingStatus
	if err := h.dbClient.UpsertImage(c, dbImage); err != nil {
		return nil, err
	}

	importImageInfo.JobName = job.Name
	if err := h.dbClient.UpsertImageImportJob(c, importImageInfo); err != nil {
		return nil, err
	}

	return &ImportImageResponse{}, nil
}

// retryDispatchImportImageJob retries a failed or stuck import job by dispatching a new one.
// Retrieves existing image and import job records, then creates a fresh Kubernetes job.
func (h *ImageHandler) retryDispatchImportImageJob(c *gin.Context) (interface{}, error) {
	id, err := parseImageID(c.Param("id"))
	if err != nil {
		return nil, err
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.ImageImportKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	existImage, err := h.dbClient.GetImage(c, id)
	if err != nil {
		return nil, err
	}
	if existImage == nil {
		return nil, commonerrors.NewNotFound("get image by id", strconv.Itoa(int(id)))
	}

	importImage, err := h.dbClient.GetImportImageByImageID(c, existImage.ID)
	if err != nil {
		return nil, err
	}
	if importImage == nil {
		return nil, commonerrors.NewNotFound("get import image by id", strconv.Itoa(int(id)))
	}

	// Get user secret if needed for retry
	var userSecret *corev1.Secret
	if existImage.SecretID != "" {
		userSecret, err = h.getAndValidateImageSecret(c.Request.Context(), existImage.SecretID)
		if err != nil {
			return nil, err
		}
	}

	job, err := h.dispatchImportImageJob(c, existImage, importImage, userSecret)
	if err != nil {
		return nil, err
	}

	existImage.Status = common.ImageImportingStatus
	if err := h.dbClient.UpsertImage(c, existImage); err != nil {
		return nil, err
	}

	importImage.JobName = job.Name
	if err := h.dbClient.UpsertImageImportJob(c, importImage); err != nil {
		return nil, err
	}
	return nil, nil
}

// dispatchImportImageJob creates and submits a Kubernetes Job to import an image.
// Configures the job with source/dest info, registry auth, and image pull secrets.
// If userSecret is provided, merges system and user auth configs into a ConfigMap.
func (h *ImageHandler) dispatchImportImageJob(c *gin.Context, image *model.Image, info *model.ImageImportJob, userSecret *corev1.Secret) (*batchv1.Job, error) {
	jobName := generateImportImageJobName(image.ID)
	imagePullSecrets, err := h.listImagePullSecretsName(c, h.Client, common.PrimusSafeNamespace)
	if err != nil {
		return nil, err
	}

	// If user secret is provided, merge auth configs and create ConfigMap
	var authConfigMap *corev1.ConfigMap
	var authConfigMapName string
	if userSecret != nil {
		cm, err := h.createMergedAuthConfigMap(c.Request.Context(), jobName, userSecret)
		if err != nil {
			return nil, err
		}
		authConfigMap = cm
		authConfigMapName = cm.Name
	}

	job, err := newImportImageJob(
		image.ID,
		jobName,
		SyncerImage,
		imagePullSecrets,
		&ImportImageEnv{
			SourceImageName: info.SrcTag,
			DestImageName:   info.DstName,
			OsArch:          fmt.Sprintf(OSArchFormat, info.Os, info.Arch),
			Os:              info.Os,
			Arch:            info.Arch,
		},
		image.CreatedBy,
		authConfigMapName, // Pass ConfigMap name (empty if no user secret)
	)
	if err != nil {
		return nil, err
	}

	if err := h.Client.Create(c.Request.Context(), job); err != nil {
		return nil, err
	}

	// Set ConfigMap owner reference to Job for auto cleanup (after Job is created, now we have UID)
	if authConfigMap != nil {
		authConfigMap.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "batch/v1",
				Kind:       "Job",
				Name:       job.Name,
				UID:        job.UID,
			},
		}
		_ = h.Client.Update(c.Request.Context(), authConfigMap)
	}

	return job, nil
}

// newImportImageJob constructs a Kubernetes Job spec for importing an image.
// Configures container with sync-image tool, environment variables, and registry authentication.
// If authConfigMapName is provided, mounts ConfigMap; otherwise mounts system secret directly.
func newImportImageJob(
	imageId int32,
	jobName,
	syncImage string,
	imagePullSecrets []string,
	env *ImportImageEnv,
	userName string,
	authConfigMapName string,
) (*batchv1.Job, error) {
	namespace := common.PrimusSafeNamespace
	envs := defaultSyncImageEnv()

	// Set platform-specific override or all-platform mode
	if len(env.OsArch) > 0 && env.OsArch != OsArchAll {
		envs[OverrideOS] = env.Os
		envs[OverrideArch] = env.Arch
	} else {
		envs[All] = StringValueTrue
	}
	envs[SrcImageEnv] = env.SourceImageName
	envs[DestImageEnv] = env.DestImageName

	// Build volumes based on whether ConfigMap is provided
	volumes, volumeMounts := buildAuthVolumes(authConfigMapName)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Annotations: map[string]string{
				v1.UserNameAnnotation:          userName,
				ImportImageTargetAnnotationKey: env.DestImageName,
				ImportImageSourceAnnotationKey: env.SourceImageName,
				ImportImageOSArchAnnotationKey: env.OsArch,
			},
			Labels: map[string]string{
				ImportImageJobLabelKey:     StringValueTrue,
				ImportImageImageIdLabelKey: strconv.Itoa(int(imageId)),
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: pointer.Int32(24 * 60 * 60),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: namespace,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:         "import-image",
							Image:        syncImage,
							Env:          transEnvMapToEnv(envs),
							VolumeMounts: volumeMounts,
						},
					},
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: buildImagePullSecrets(imagePullSecrets),
					Volumes:          volumes,
				},
			},
		},
	}
	return job, nil
}

// buildAuthVolumes builds volumes and volume mounts for auth config.
// If authConfigMapName is provided, mounts the ConfigMap; otherwise mounts system secret.
func buildAuthVolumes(authConfigMapName string) ([]corev1.Volume, []corev1.VolumeMount) {
	var volumes []corev1.Volume

	if authConfigMapName == "" {
		volumes = []corev1.Volume{
			{
				Name: "registry-auth",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: common.ImageImportSecretName,
					},
				},
			},
		}
	} else {
		volumes = []corev1.Volume{
			{
				Name: "registry-auth",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: authConfigMapName,
						},
					},
				},
			},
		}
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "registry-auth",
			ReadOnly:  true,
			MountPath: "/root/.docker",
		},
	}

	return volumes, volumeMounts
}

// createMergedAuthConfigMap creates a ConfigMap containing merged auth configs from system and user secrets.
// The merged config combines the "auths" sections from both secrets, with user auths overriding system auths for the same registry.
// Returns the created ConfigMap object for setting OwnerReference later.
func (h *ImageHandler) createMergedAuthConfigMap(ctx context.Context, jobName string, userSecret *corev1.Secret) (*corev1.ConfigMap, error) {
	namespace := common.PrimusSafeNamespace

	// Get system secret
	systemSecret := &corev1.Secret{}
	if err := h.Client.Get(ctx, client.ObjectKey{
		Name:      common.ImageImportSecretName,
		Namespace: namespace,
	}, systemSecret); err != nil {
		klog.ErrorS(err, "failed to get system secret", "secretName", common.ImageImportSecretName)
		return nil, fmt.Errorf("failed to get system secret: %w", err)
	}

	// Parse system auth config
	systemAuths := make(map[string]interface{})
	if configData, ok := systemSecret.Data["config.json"]; ok {
		var systemConfig struct {
			Auths map[string]interface{} `json:"auths"`
		}
		if err := json.Unmarshal(configData, &systemConfig); err == nil {
			systemAuths = systemConfig.Auths
		}
	}

	// Parse user auth config (userSecret is already fetched, no need to Get again)
	userAuths := make(map[string]interface{})
	if configData, ok := userSecret.Data[".dockerconfigjson"]; ok {
		var userConfig struct {
			Auths map[string]interface{} `json:"auths"`
		}
		if err := json.Unmarshal(configData, &userConfig); err == nil {
			userAuths = userConfig.Auths
		}
	}

	// Merge auths (user auths override system auths)
	mergedAuths := make(map[string]interface{})
	for k, v := range systemAuths {
		mergedAuths[k] = v
	}
	for k, v := range userAuths {
		mergedAuths[k] = v
	}

	// Create merged config JSON
	mergedConfig := map[string]interface{}{
		"auths": mergedAuths,
	}
	mergedConfigJSON, err := json.Marshal(mergedConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged config: %w", err)
	}

	// Create ConfigMap
	configMapName := fmt.Sprintf("%s-auth", jobName)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				ImportImageJobLabelKey: StringValueTrue,
			},
		},
		Data: map[string]string{
			"config.json": string(mergedConfigJSON),
		},
	}

	if err := h.Client.Create(ctx, configMap); err != nil {
		klog.ErrorS(err, "failed to create auth ConfigMap", "configMapName", configMapName)
		return nil, fmt.Errorf("failed to create auth ConfigMap: %w", err)
	}

	klog.V(4).InfoS("created merged auth ConfigMap", "configMapName", configMapName)
	return configMap, nil
}

// buildImagePullSecrets converts a list of secret names to LocalObjectReference slice.
// Helper function to construct ImagePullSecrets for pod spec.
func buildImagePullSecrets(secretNames []string) []corev1.LocalObjectReference {
	secrets := make([]corev1.LocalObjectReference, 0, len(secretNames))
	for _, name := range secretNames {
		secrets = append(secrets, corev1.LocalObjectReference{Name: name})
	}
	return secrets
}

// transEnvMapToEnv converts a string map to Kubernetes EnvVar slice.
// Each map entry becomes a single environment variable in the container.
func transEnvMapToEnv(envMap map[string]string) []corev1.EnvVar {
	envs := make([]corev1.EnvVar, 0, len(envMap))
	for k, v := range envMap {
		envs = append(envs, corev1.EnvVar{Name: k, Value: v})
	}
	return envs
}

// defaultSyncImageEnv returns default environment variables for the sync-image container.
// Configures source/destination settings, TLS verification, and API service endpoint.
func defaultSyncImageEnv() map[string]string {
	return map[string]string{
		DEBUG:           StringValueTrue,
		GlobalTLSVerify: StringValueTrue,
		OverrideArch:    "",
		OverrideOS:      "",
		CommandTimeout:  "0",
		SourceType:      "docker",
		DestinationType: "docker",
		All:             "false",
		SrcUserName:     "",
		SrcPassword:     "",
		TLSVerify:       "true",
		DestUserName:    "",
		DestPassword:    "",
		DestTLSVerify:   "true",
		SrcImageEnv:     "",
		DestImageEnv:    "",
		UpstreamDomain:  ApiServiceName,
	}
}

// generateImportImageJobName generates a unique job name for an import job.
// Uses image ID and timestamp hash to ensure uniqueness across retries.
func generateImportImageJobName(imageId int32) string {
	return fmt.Sprintf("imptimg-%d-%016x", imageId, xxhash.Sum64String(time.Now().String()))
}

// getImportImageInfo validates and prepares image metadata for import operation.
// Checks source image existence, calculates size, and determines destination name.
// If userSecret is provided, it will be used for authenticating against the source registry.
func (h *ImageHandler) getImportImageInfo(ctx context.Context, req *ImportImageServiceRequest, userSecret *corev1.Secret) (*ImportImageMetaInfo, error) {
	imageInfo := &ImportImageMetaInfo{
		SourceImageName: req.Source,
		OsArch:          fmt.Sprintf(OSArchFormat, DefaultOS, DefaultArch),
		Os:              DefaultOS,
		Arch:            DefaultArch,
		Status:          common.ImageImportingStatus,
	}

	if req.SourceRegistry != "" {
		imageInfo.SourceImageName = fmt.Sprintf("%s/%s", req.SourceRegistry, req.Source)
	}

	// Get default push registry
	defaultPushRegistry, err := h.dbClient.GetDefaultRegistryInfo(ctx)
	if err != nil {
		klog.ErrorS(err, "GetPushRegistryInfo error")
		return nil, commonerrors.NewInternalError("Database Error")
	}
	if defaultPushRegistry == nil {
		return nil, commonerrors.NewBadRequest("Default push registry not exist. Please contact your administrator")
	}

	imageInfo.DestImageName, err = generateTargetImageName(defaultPushRegistry.URL, imageInfo.SourceImageName)
	if err != nil {
		return nil, err
	}

	if err := h.checkImageExistsUsingLibrary(ctx, req.Source, imageInfo, userSecret); err != nil {
		return nil, err
	}

	return imageInfo, nil
}

// generateTargetImageName constructs the destination image name in the target registry.
// Strips source registry host and prepends target registry with sync-image project namespace.
func generateTargetImageName(targetRegistryHost, sourceImage string) (string, error) {
	slc := strings.SplitN(sourceImage, "/", 2)
	if len(slc) != 2 {
		return "", fmt.Errorf("source image name is invalid")
	}
	repoAndTag := slc[1]
	return fmt.Sprintf("%s/%s/%s", targetRegistryHost, SyncImageProject, repoAndTag), nil
}

// existImageValid checks if an image with the given tag already exists in database.
// Returns the existing image ID if found, otherwise returns 0.
func (h *ImageHandler) existImageValid(c context.Context, destImageName string) (int32, error) {
	existImage, err := h.dbClient.GetImageByTag(c, destImageName)
	if err != nil {
		klog.ErrorS(err, "get image by image tag error")
		return 0, commonerrors.NewInternalError("Database Error")
	}
	if existImage != nil {
		return existImage.ID, nil
	}
	return 0, nil
}

// checkImageExistsUsingLibrary verifies source image exists and retrieves its size.
// Uses container image library to inspect manifest and calculate total layer sizes.
// If userSecret is provided, it will be used for authenticating against the source registry.
func (h *ImageHandler) checkImageExistsUsingLibrary(ctx context.Context, imageName string, imageInfo *ImportImageMetaInfo, userSecret *corev1.Secret) error {
	list := strings.Split(imageInfo.OsArch, "/")
	if len(list) != 2 {
		return commonerrors.NewBadRequest("invalid os/arch format")
	}
	hostName := strings.Split(imageInfo.SourceImageName, "/")[0]
	os, arch := list[0], list[1]

	sysCtx, err := h.getImageSystemCtx(ctx, hostName, imageName, userSecret)
	if err != nil {
		klog.Errorf("Error getting system context: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Getting Registry Auth Error: %s", err))
	}

	imageName = fmt.Sprintf("docker://%s", imageName)
	ref, err := alltransports.ParseImageName(imageName)
	if err != nil {
		klog.Errorf("Error parsing reference: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Parsing Reference Error: %s", err))
	}

	src, err := ref.NewImageSource(ctx, sysCtx)
	if err != nil {
		klog.Errorf("Image not found or inaccessible: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Image not found or inaccessible: %s", err))
	}
	defer src.Close()

	manifest, manifestType, err := src.GetManifest(ctx, nil)
	if err != nil {
		klog.Errorf("Getting Manifest Error: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Getting Manifest Error: %s", err))
	}

	totalSize := int64(0)

	// Handle multi-platform manifests (index/list)
	if manifestType == imagespecv1.MediaTypeImageIndex || manifestType == manifestv5.DockerV2ListMediaType {
		totalSize += int64(len(manifest))
		targetDigest, err := h.extractPlatformDigest(manifest, manifestType, os, arch)
		if err != nil {
			return err
		}

		manifest, manifestType, err = src.GetManifest(ctx, &targetDigest)
		if err != nil {
			klog.Errorf("Error getting platform-specific manifest: %s", err)
			return commonerrors.NewInternalError(fmt.Sprintf("Getting platform-specific manifest Error: %s", err))
		}
	}

	// Calculate total size from platform-specific manifest
	totalSize += int64(len(manifest))
	layerSize, err := h.calculateManifestSize(manifest, manifestType)
	if err != nil {
		return err
	}
	totalSize += layerSize

	imageInfo.Size = totalSize
	klog.Infof("Image %s exists, size: %d", imageName, totalSize)
	return nil
}

// extractPlatformDigest extracts the digest for a specific OS/arch from a multi-platform manifest.
// Supports both OCI Image Index and Docker Manifest List formats.
func (h *ImageHandler) extractPlatformDigest(manifest []byte, manifestType string, os, arch string) (imagedigest.Digest, error) {
	if manifestType == imagespecv1.MediaTypeImageIndex {
		var index imagespecv1.Index
		if err := json.Unmarshal(manifest, &index); err != nil {
			klog.Errorf("Error parsing OCI index: %s", err)
			return "", commonerrors.NewInternalError(fmt.Sprintf("Parsing OCI index Error: %s", err))
		}

		for _, m := range index.Manifests {
			if m.Platform != nil && m.Platform.OS == os && m.Platform.Architecture == arch {
				return m.Digest, nil
			}
		}
	} else {
		var schema2List manifestv5.Schema2List
		if err := json.Unmarshal(manifest, &schema2List); err != nil {
			klog.Errorf("Error parsing Docker manifest list: %s", err)
			return "", commonerrors.NewInternalError(fmt.Sprintf("Parsing Docker manifest list Error: %s", err))
		}

		for _, m := range schema2List.Manifests {
			if m.Platform.OS == os && m.Platform.Architecture == arch {
				return m.Digest, nil
			}
		}
	}

	return "", commonerrors.NewInternalError(fmt.Sprintf("No matching manifest found for OS %s and architecture %s", os, arch))
}

// calculateManifestSize calculates total size of all layers in a manifest.
// Supports both OCI Image Manifest and Docker v2 Schema 2 formats.
func (h *ImageHandler) calculateManifestSize(manifest []byte, manifestType string) (int64, error) {
	totalSize := int64(0)

	switch manifestType {
	case imagespecv1.MediaTypeImageManifest:
		var v1Manifest imagespecv1.Manifest
		if err := json.Unmarshal(manifest, &v1Manifest); err != nil {
			klog.Errorf("Error parsing OCI manifest: %s", err)
			return 0, commonerrors.NewInternalError(fmt.Sprintf("Parsing OCI manifest Error: %s", err))
		}
		for _, layer := range v1Manifest.Layers {
			totalSize += layer.Size
		}
		totalSize += v1Manifest.Config.Size

	case manifestv5.DockerV2Schema2MediaType:
		var v2Manifest manifestv5.Schema2
		if err := json.Unmarshal(manifest, &v2Manifest); err != nil {
			klog.Errorf("Error parsing Docker manifest: %s", err)
			return 0, commonerrors.NewInternalError(fmt.Sprintf("Parsing Docker manifest Error: %s", err))
		}
		for _, layer := range v2Manifest.LayerInfos() {
			totalSize += layer.Size
		}
		totalSize += v2Manifest.ConfigInfo().Size

	default:
		return 0, commonerrors.NewInternalError(fmt.Sprintf("Unsupported manifest type: %s", manifestType))
	}

	return totalSize, nil
}

// getImageSystemCtx creates a SystemContext with authentication for accessing a registry.
// Fetches registry credentials from user secret (if provided), database, or Docker Hub token for public registries.
// Priority: user secret > database registry_info > Docker Hub anonymous token
func (h *ImageHandler) getImageSystemCtx(ctx context.Context, hostName string, imageName string, userSecret *corev1.Secret) (*v5types.SystemContext, error) {
	sysCtx := &v5types.SystemContext{DockerInsecureSkipTLSVerify: v5types.OptionalBoolTrue}

	// If user provided a secret, try to use it first
	if userSecret != nil {
		authConfig, err := h.extractAuthFromSecret(hostName, userSecret)
		if err != nil {
			klog.ErrorS(err, "failed to get auth from user secret", "secretName", userSecret.Name)
			// Fall through to try other auth methods
		} else if authConfig != nil {
			sysCtx.DockerAuthConfig = authConfig
			return sysCtx, nil
		}
	}

	// Try to get auth from database registry_info
	accountInfo, err := h.dbClient.GetRegistryInfoByUrl(ctx, hostName)
	if err != nil {
		// Special handling for Docker Hub
		if strings.HasSuffix(hostName, "docker.io") {
			return h.getDockerHubSystemCtx(ctx, imageName)
		}
		klog.Errorf("Error getting registry info, err is: %s", err)
		return nil, err
	}

	if accountInfo != nil {
		authConfig, err := h.decryptRegistryAuth(accountInfo)
		if err != nil {
			return nil, err
		}
		sysCtx.DockerAuthConfig = authConfig
	}

	return sysCtx, nil
}

// getDockerHubSystemCtx creates a SystemContext with Docker Hub authentication token.
// Fetches an anonymous token from Docker Hub auth service for pulling public images.
func (h *ImageHandler) getDockerHubSystemCtx(ctx context.Context, imageName string) (*v5types.SystemContext, error) {
	// Extract image path: library/alpine:latest -> library/alpine
	item := strings.Join(strings.Split(imageName, "/")[1:], "/")
	imagePath := strings.Split(item, ":")[0]

	token, err := h.fetchDockerToken(ctx, imagePath)
	if err != nil {
		klog.Errorf("Error fetching token, err is: %s", err)
		return nil, err
	}

	return &v5types.SystemContext{
		DockerInsecureSkipTLSVerify: v5types.OptionalBoolTrue,
		DockerBearerRegistryToken:   token,
	}, nil
}

// extractAuthFromSecret extracts authentication for a specific registry from a secret object.
// Returns nil if the secret doesn't contain auth for the specified hostName.
func (h *ImageHandler) extractAuthFromSecret(hostName string, secret *corev1.Secret) (*v5types.DockerAuthConfig, error) {
	// Parse user secret auths (stored in ".dockerconfigjson" key for kubernetes.io/dockerconfigjson type)
	configData, ok := secret.Data[".dockerconfigjson"]
	if !ok {
		return nil, nil
	}

	var dockerConfig struct {
		Auths map[string]struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Auth     string `json:"auth"`
		} `json:"auths"`
	}

	if err := json.Unmarshal(configData, &dockerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse user secret: %w", err)
	}

	// Look for auth matching the hostName
	for host, authItem := range dockerConfig.Auths {
		matched := strings.Contains(host, hostName)
		if !matched && strings.HasSuffix(hostName, "docker.io") {
			matched = strings.Contains(host, "docker.io")
		}
		if !matched {
			continue
		}

		// If username/password are provided directly, use them
		if authItem.Username != "" && authItem.Password != "" {
			return &v5types.DockerAuthConfig{
				Username: authItem.Username,
				Password: authItem.Password,
			}, nil
		}
		// Otherwise decode from auth field (base64 of "username:password")
		if authItem.Auth != "" {
			decoded, err := base64.StdEncoding.DecodeString(authItem.Auth)
			if err != nil {
				decoded, err = base64.URLEncoding.DecodeString(authItem.Auth)
				if err != nil {
					continue
				}
			}
			parts := strings.SplitN(string(decoded), ":", 2)
			if len(parts) == 2 {
				return &v5types.DockerAuthConfig{
					Username: parts[0],
					Password: parts[1],
				}, nil
			}
		}
	}

	klog.V(4).Infof("No matching auth found in user secret %s for host %s", secret.Name, hostName)
	return nil, nil
}

// decryptRegistryAuth decrypts username and password from registry account info.
// Returns a DockerAuthConfig with decrypted credentials for registry authentication.
func (h *ImageHandler) decryptRegistryAuth(accountInfo *model.RegistryInfo) (*v5types.DockerAuthConfig, error) {
	password := ""
	if accountInfo.Password != "" {
		var err error
		password, err = crypto.NewCrypto().Decrypt(accountInfo.Password)
		if err != nil {
			klog.Errorf("Error decrypting password, err is: %s", err)
			return nil, err
		}
	}

	userName := ""
	if accountInfo.Username != "" {
		var err error
		userName, err = crypto.NewCrypto().Decrypt(accountInfo.Username)
		if err != nil {
			klog.Errorf("Error decrypting username, err is: %s", err)
			return nil, err
		}
	}

	return &v5types.DockerAuthConfig{
		Username: userName,
		Password: password,
	}, nil
}

// fetchDockerToken retrieves an anonymous access token from Docker Hub auth service.
// The token is used for pulling public images without authentication.
func (h *ImageHandler) fetchDockerToken(_ context.Context, imagePath string) (string, error) {
	url := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", imagePath)
	resp, err := h.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("error fetching token: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(resp.Body, &tokenResponse); err != nil {
		return "", fmt.Errorf("error parsing JSON: %w", err)
	}

	return tokenResponse.Token, nil
}

// parseImageID parses and validates an image ID from a string parameter.
// Returns an error if the ID is empty or not a valid integer.
func parseImageID(idStr string) (int32, error) {
	if idStr == "" {
		return 0, commonerrors.NewBadRequest("image id is empty")
	}
	imageID, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, commonerrors.NewBadRequest("invalid image id")
	}
	return int32(imageID), nil
}

// deserializeParams converts a serialized parameter string into a slice of Parameter objects.
// It parses the string representation of parameters (format: {name:value,name2:value2}) and converts them to structured format.
func deserializeParams(strInput string) []v1.Parameter {
	if len(strInput) <= 1 {
		return nil
	}
	// Remove surrounding braces: {workload:xxx,image:yyy} → workload:xxx,image:yyy
	strInput = strInput[1 : len(strInput)-1]
	splitParams := strings.Split(strInput, ",")
	var result []v1.Parameter
	for _, p := range splitParams {
		// Trim spaces and quotes (handle "label:value" format)
		p = strings.TrimSpace(p)
		p = strings.Trim(p, "\"")
		parts := strings.SplitN(p, ":", 2)
		if len(parts) == 2 {
			result = append(result, v1.Parameter{
				Name:  parts[0],
				Value: parts[1],
			})
		}
	}
	return result
}

// buildExportImageJobQuery builds SQL query for export image jobs from ops_job table.
func buildExportImageJobQuery(query *ImageServiceRequest) (sqrl.Sqlizer, []string) {
	dbTags := dbClient.GetOpsJobFieldTags()

	// Build WHERE clause
	dbSql := sqrl.And{
		sqrl.Eq{dbClient.GetFieldTag(dbTags, "Type"): string(v1.OpsJobExportImageType)},
		sqrl.Eq{dbClient.GetFieldTag(dbTags, "IsDeleted"): false},
	}

	// Filter by user name if specified
	if query.UserName != "" {
		dbSql = append(dbSql, sqrl.Eq{dbClient.GetFieldTag(dbTags, "UserName"): query.UserName})
	}

	// Filter by ready status (only show succeeded jobs)
	if query.Ready {
		dbSql = append(dbSql, sqrl.Eq{dbClient.GetFieldTag(dbTags, "Phase"): string(v1.OpsJobSucceeded)})
	}

	// Filter by workload ID if specified (using text pattern matching)
	if query.Workload != "" {
		// Pattern: workload:xxx followed by comma or closing brace
		pattern := fmt.Sprintf("workload:%s[,}]", query.Workload)
		dbSql = append(dbSql, sqrl.Expr("inputs::text ~ ?", pattern))
	}

	// Build ORDER BY clause
	// Note: ops_job table uses "creation_time", not "created_at" like image table
	orderByField := dbClient.GetFieldTag(dbTags, "CreationTime")
	// Ignore query.OrderBy as it may contain image table field names
	order := "DESC"
	if query.Order != "" {
		order = strings.ToUpper(query.Order)
	}
	orderBy := []string{fmt.Sprintf("%s %s", orderByField, order)}

	return dbSql, orderBy
}

// buildPrewarmImageJobQuery builds SQL query for prewarm image jobs from ops_job table.
func buildPrewarmImageJobQuery(query *ImageServiceRequest) (sqrl.Sqlizer, []string) {
	dbTags := dbClient.GetOpsJobFieldTags()

	dbSql := sqrl.And{
		sqrl.Eq{dbClient.GetFieldTag(dbTags, "Type"): string(v1.OpsJobPrewarmType)},
		sqrl.Eq{dbClient.GetFieldTag(dbTags, "IsDeleted"): false},
	}

	if query.UserName != "" {
		dbSql = append(dbSql, sqrl.Eq{dbClient.GetFieldTag(dbTags, "UserName"): query.UserName})
	}

	if query.Ready {
		dbSql = append(dbSql, sqrl.Eq{dbClient.GetFieldTag(dbTags, "Phase"): string(v1.OpsJobSucceeded)})
	}

	if query.Image != "" {
		pattern := fmt.Sprintf("image:%s[,}]", query.Image)
		dbSql = append(dbSql, sqrl.Expr("inputs::text ~ ?", pattern))
	}

	if query.Workspace != "" {
		pattern := fmt.Sprintf("workspace:%s[,}]", query.Workspace)
		dbSql = append(dbSql, sqrl.Expr("inputs::text ~ ?", pattern))
	}

	if query.Status != "" {
		if query.Status == "Running" {
			dbSql = append(dbSql, sqrl.Eq{dbClient.GetFieldTag(dbTags, "Phase"): query.Status})
		} else {
			statusFilter := fmt.Sprintf(`[{"name": "status", "value": "%s"}]`, query.Status)
			dbSql = append(dbSql, sqrl.Expr("outputs::jsonb @> ?::jsonb", statusFilter))
		}
	}

	orderByField := dbClient.GetFieldTag(dbTags, "CreationTime")
	order := "DESC"
	if query.Order != "" {
		order = strings.ToUpper(query.Order)
	}
	orderBy := []string{fmt.Sprintf("%s %s", orderByField, order)}

	return dbSql, orderBy
}

// convertOpsJobToPrewarmImageList converts ops_job records to PrewarmImageListItem slice.
func (h *ImageHandler) convertOpsJobToPrewarmImageList(ctx context.Context, jobs []*dbClient.OpsJob) []PrewarmImageListItem {
	result := make([]PrewarmImageListItem, 0, len(jobs))

	for _, job := range jobs {
		item := PrewarmImageListItem{
			Status:      dbutils.ParseNullString(job.Phase),
			CreatedTime: timeutil.FormatRFC3339(dbutils.ParseNullTime(job.CreationTime)),
			EndTime:     timeutil.FormatRFC3339(dbutils.ParseNullTime(job.EndTime)),
			UserName:    dbutils.ParseNullString(job.UserName),
		}

		// Parse inputs to extract image and workspace
		if len(job.Inputs) > 0 {
			inputs := deserializeParams(string(job.Inputs))
			for _, param := range inputs {
				switch param.Name {
				case v1.ParameterImage:
					item.ImageName = param.Value
				case v1.ParameterWorkspace:
					item.WorkspaceId = param.Value
				}
			}
		}

		if item.WorkspaceId != "" {
			workspace := &v1.Workspace{}
			if err := h.Get(ctx, client.ObjectKey{Name: item.WorkspaceId}, workspace); err == nil {
				item.WorkspaceName = v1.GetDisplayName(workspace)
			} else {
				item.WorkspaceName = item.WorkspaceId
				klog.V(4).ErrorS(err, "Failed to get workspace displayName, using ID as name", "workspaceId", item.WorkspaceId)
			}
		}

		// Parse outputs to extract status and prewarm_progress
		if outputsStr := dbutils.ParseNullString(job.Outputs); outputsStr != "" {
			var outputs []v1.Parameter
			if err := json.Unmarshal([]byte(outputsStr), &outputs); err == nil {
				for _, param := range outputs {
					switch param.Name {
					case "status":
						item.Status = param.Value
					case "prewarm_progress":
						item.PrewarmProgress = param.Value
					}
				}
			}
		}

		if conditionsStr := dbutils.ParseNullString(job.Conditions); conditionsStr != "" {
			var conditions []metav1.Condition
			if err := json.Unmarshal([]byte(conditionsStr), &conditions); err == nil {
				for i := len(conditions) - 1; i >= 0; i-- {
					if conditions[i].Message != "" {
						item.ErrorMessage = conditions[i].Message
						break
					}
				}
			}
		}

		result = append(result, item)
	}
	return result
}

// convertOpsJobToExportedImageList converts ops_job records to ExportedImageListItem slice.
func convertOpsJobToExportedImageList(jobs []*dbClient.OpsJob) []ExportedImageListItem {
	result := make([]ExportedImageListItem, 0, len(jobs))

	for _, job := range jobs {
		item := ExportedImageListItem{
			Status:      dbutils.ParseNullString(job.Phase),
			CreatedTime: timeutil.FormatRFC3339(dbutils.ParseNullTime(job.CreationTime)),
		}

		// Parse inputs using the standard deserializeParams function
		// Format: {workload:xxx,label:yyy,image:zzz}
		if len(job.Inputs) > 0 {
			inputs := deserializeParams(string(job.Inputs))
			for _, param := range inputs {
				switch param.Name {
				case "workload":
					item.Workload = param.Value
				case "label":
					item.Label = param.Value
				}
			}
		}

		// Parse outputs to extract target image name
		if outputsStr := dbutils.ParseNullString(job.Outputs); outputsStr != "" {
			var outputs []v1.Parameter
			if err := json.Unmarshal([]byte(outputsStr), &outputs); err == nil {
				for _, param := range outputs {
					if param.Name == "target" {
						item.ImageName = param.Value
						break
					}
				}
			}
		}

		// Parse conditions to extract log message
		if conditionsStr := dbutils.ParseNullString(job.Conditions); conditionsStr != "" {
			var conditions []metav1.Condition
			if err := json.Unmarshal([]byte(conditionsStr), &conditions); err == nil {
				// Get the most recent condition message
				for i := len(conditions) - 1; i >= 0; i-- {
					if conditions[i].Message != "" {
						item.Log = conditions[i].Message
						break
					}
				}
			}
		}

		result = append(result, item)
	}

	return result
}

// getAndValidateImageSecret gets and validates that the provided secret ID exists and is of type "image".
// Returns the secret object for reuse, avoiding multiple API calls.
func (h *ImageHandler) getAndValidateImageSecret(ctx context.Context, secretId string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := h.Client.Get(ctx, client.ObjectKey{
		Name:      secretId,
		Namespace: common.PrimusSafeNamespace,
	}, secret); err != nil {
		klog.ErrorS(err, "failed to get secret", "secretId", secretId)
		return nil, commonerrors.NewNotFound("secret", secretId)
	}

	// Check if the secret is of type "image"
	secretType := secret.Labels[v1.SecretTypeLabel]
	if secretType != string(v1.SecretImage) {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("secret %s is not an image-type secret, got type: %s", secretId, secretType))
	}

	return secret, nil
}
