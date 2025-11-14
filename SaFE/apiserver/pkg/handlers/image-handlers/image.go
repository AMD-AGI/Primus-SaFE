/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
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
		ResourceKind: common.ImageImportKind,
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
		ResourceKind: common.ImageImportKind,
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
		})
	}
	return res
}

// parseListImageQuery extracts and validates query parameters for listing images.
// Sets default values for pagination, ordering, and filters if not provided.
func parseListImageQuery(c *gin.Context) (*ImageServiceRequest, error) {
	query := &ImageServiceRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
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
		ResourceKind: common.ImageImportKind,
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
		ResourceKind: common.ImageImportKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	userName := c.GetString(common.UserName)
	imageInfo, err := h.getImportImageInfo(c, body)
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

	// Dispatch Kubernetes job
	job, err := h.dispatchImportImageJob(c, dbImage, importImageInfo)
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
		ResourceKind: common.ImageImportKind,
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

	job, err := h.dispatchImportImageJob(c, existImage, importImage)
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
func (h *ImageHandler) dispatchImportImageJob(c *gin.Context, image *model.Image, info *model.ImageImportJob) (*batchv1.Job, error) {
	jobName := generateImportImageJobName(image.ID)
	imagePullSecrets, err := h.listImagePullSecretsName(c, h.Client, DefaultNamespace)
	if err != nil {
		return nil, err
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
	)
	if err != nil {
		return nil, err
	}

	if err := h.Client.Create(c.Request.Context(), job); err != nil {
		return nil, err
	}
	return job, nil
}

// newImportImageJob constructs a Kubernetes Job spec for importing an image.
// Configures container with sync-image tool, environment variables, and registry authentication.
func newImportImageJob(
	imageId int32,
	jobName,
	syncImage string,
	imagePullSecrets []string,
	env *ImportImageEnv,
	userName string,
) (*batchv1.Job, error) {
	namespace := DefaultNamespace
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
							Name:  "import-image",
							Image: syncImage,
							Env:   transEnvMapToEnv(envs),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "registry-auth",
									ReadOnly:  true,
									MountPath: "/root/.docker",
								},
							},
						},
					},
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: buildImagePullSecrets(imagePullSecrets),
					Volumes: []corev1.Volume{
						{
							Name: "registry-auth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: ImageImportSecretName,
								},
							},
						},
					},
				},
			},
		},
	}
	return job, nil
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
func (h *ImageHandler) getImportImageInfo(c context.Context, req *ImportImageServiceRequest) (*ImportImageMetaInfo, error) {
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
	defaultPushRegistry, err := h.dbClient.GetDefaultRegistryInfo(c)
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

	if err := h.checkImageExistsUsingLibrary(c, req.Source, imageInfo); err != nil {
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
func (h *ImageHandler) checkImageExistsUsingLibrary(ctx context.Context, imageName string, imageInfo *ImportImageMetaInfo) error {
	list := strings.Split(imageInfo.OsArch, "/")
	if len(list) != 2 {
		return commonerrors.NewBadRequest("invalid os/arch format")
	}
	hostName := strings.Split(imageInfo.SourceImageName, "/")[0]
	os, arch := list[0], list[1]

	sysCtx, err := h.getImageSystemCtx(ctx, hostName, imageName)
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
// Fetches registry credentials from database or Docker Hub token for public registries.
func (h *ImageHandler) getImageSystemCtx(ctx context.Context, hostName string, imageName string) (*v5types.SystemContext, error) {
	sysCtx := &v5types.SystemContext{DockerInsecureSkipTLSVerify: v5types.OptionalBoolTrue}

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
