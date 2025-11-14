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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

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
	err = h.dbClient.UpsertImage(c, image)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *ImageHandler) deleteImage(c *gin.Context) (interface{}, error) {
	imageIDStr := c.Param("id")
	if imageIDStr == "" {
		return nil, commonerrors.NewBadRequest("image id is empty")
	}
	imageID, err := strconv.Atoi(imageIDStr)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid image id")
	}
	existImage, err := h.dbClient.GetImage(c, int32(imageID))
	if err != nil {
		return nil, err
	}
	if existImage == nil {
		return nil, nil
	}
	err = h.dbClient.DeleteImage(c, int32(imageID), existImage.DeletedBy)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

func (h *ImageHandler) listImage(c *gin.Context) (interface{}, error) {
	query, err := parseListImageQuery(c)
	if err != nil {
		klog.ErrorS(err, "fail to parseListImageQuery")
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
	results := &GetImageResponse{
		TotalCount: count,
	}
	results.Items = cvtImageToResponse(images, DefaultOS, DefaultArch)
	return results, nil
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

func cvtImageToFlatResponse(images []*model.Image) []Image {
	res := make([]Image, 0)
	for _, image := range images {
		res = append(res, Image{
			Id:          int32(image.ID),
			Tag:         image.Tag,
			Description: image.Description,
			CreatedBy:   image.CreatedBy,
			CreatedAt:   image.CreatedAt.Unix(),
		})
	}
	return res
}

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

func (h *ImageHandler) getImportingDetail(ctx *gin.Context) (*ImportDetailResponse, error) {
	idStr := ctx.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid id: " + err.Error())
	}
	existImage, err := h.dbClient.GetImage(ctx, int32(id))
	if err != nil {
		return nil, err
	}
	if existImage == nil {
		return nil, commonerrors.NewNotFound("get image by id", idStr)
	}
	importImage, err := h.dbClient.GetImportImageByImageID(ctx, existImage.ID)
	if err != nil {
		return nil, err
	}
	if importImage == nil {
		return nil, commonerrors.NewNotFound("get import image by id", idStr)
	}
	return &ImportDetailResponse{
		LayersDetail: importImage.Layer,
	}, nil
}

func cvtImageToResponse(images []*model.Image, os, arch string) []GetImageResponseItem {
	repoMap := map[string]int{}
	res := make([]GetImageResponseItem, 0)
	for _, image := range images {
		var RegistryHost, repo, tag string
		imageTag := image.Tag
		if spn := strings.SplitN(imageTag, "/", 2); len(spn) == 2 {
			RegistryHost = spn[0]
			if slc := strings.SplitN(spn[1], ":", 2); len(slc) == 2 {
				repo = slc[0]
				tag = slc[1]
			} else {
				klog.Errorf("image:%s is invalid, skip", imageTag)
				continue
			}
		} else {
			klog.Errorf("image:%s is invalid, skip", imageTag)
			continue
		}

		fullUrl := strings.Join([]string{RegistryHost, repo}, "/")
		artifact := ArtifactItem{
			ImageTag:    tag,
			Description: image.Description,
			CreatedTime: timeutil.FormatRFC3339(image.CreatedAt),
			UserName:    image.CreatedBy,
			Status:      image.Status,
			Id:          image.ID,
			IncludeType: image.Source,
		}

		if image.RelationDigest != nil {
			arch := fmt.Sprintf(OSArchFormat, os, arch)
			relationDigestMap := make(map[string]*RelationDigest)
			if err := decodeJsonb(image.RelationDigest, &relationDigestMap); err == nil && relationDigestMap[arch] != nil {
				artifact.Size = relationDigestMap[arch].Size
				artifact.Arch = arch
				artifact.Os = os
				artifact.Digest = relationDigestMap[arch].Digest
			}
		}

		if index, ok := repoMap[fullUrl]; !ok {
			// if not in repoMap, it is a new repo
			res = append(res, GetImageResponseItem{
				RegistryHost: RegistryHost,
				Repo:         repo,
				Artifacts: []ArtifactItem{
					artifact,
				},
			})
			repoMap[fullUrl] = len(res) - 1
		} else {
			res[index].Artifacts = append(res[index].Artifacts, artifact)
		}
	}
	return res
}

func decodeJsonb(data map[string]interface{}, to interface{}) error {
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(jsonBytes, to)
}

func (h *ImageHandler) importImage(c *gin.Context) (interface{}, error) {
	var err error
	resp := &ImportImageResponse{}
	body := &ImportImageServiceRequest{}
	if err = c.ShouldBindJSON(&body); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	userName := c.GetString(common.UserName)

	imageInfo, err := h.getImportImageInfo(c, body)
	if err != nil {
		return nil, err
	}

	existImageID, err := h.existImageVlid(c, imageInfo.DestImageName)
	if err != nil {
		return nil, err
	}
	if existImageID != 0 {
		resp.AlreadyImageID = existImageID
		resp.Message = "Image already existed. We don't need to import it again"
		return resp, nil
	}

	importImageEnv := &ImportImageEnv{
		SourceImageName: imageInfo.SourceImageName,
		DestImageName:   imageInfo.DestImageName,
		OsArch:          imageInfo.OsArch,
		Description:     fmt.Sprintf("Import from %s", imageInfo.SourceImageName),
	}

	relationDigest := map[string]interface{}{}
	defaultDigestItem := &RelationDigest{
		Digest: "",
		Size:   imageInfo.Size,
	}
	relationDigest[importImageEnv.OsArch] = defaultDigestItem
	dbImage := &model.Image{
		Tag:            imageInfo.DestImageName,
		CreatedBy:      userName,
		CreatedAt:      time.Now().UTC(),
		Description:    fmt.Sprintf("Import from %s", importImageEnv.SourceImageName),
		Status:         common.ImageImportPendingStatus,
		RelationDigest: relationDigest,
		Source:         "import",
	}
	if err := h.dbClient.UpsertImage(c, dbImage); err != nil {
		return nil, err
	}

	importImageInfo := &model.ImageImportJob{
		SrcTag:    imageInfo.SourceImageName,
		DstName:   imageInfo.DestImageName,
		Os:        importImageEnv.Os,
		Arch:      importImageEnv.Arch,
		CreatedAt: time.Now().UTC(),
		ImageID:   dbImage.ID,
	}

	if err := h.dbClient.UpsertImageImportJob(c, importImageInfo); err != nil {
		return nil, err
	}

	job, err := h.dispatchImportImageJob(c, dbImage, importImageInfo)
	if err != nil {
		return nil, err
	}
	dbImage.Status = common.ImageImportingStatus
	if err := h.dbClient.UpsertImage(c, dbImage); err != nil {
		return nil, err
	}
	importImageInfo.JobName = job.Name
	if err := h.dbClient.UpsertImageImportJob(c, importImageInfo); err != nil {
		return nil, err
	}
	return resp, nil
}

func (h *ImageHandler) retryDispatchImportImageJob(c *gin.Context) (interface{}, error) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid id: " + err.Error())
	}
	existImage, err := h.dbClient.GetImage(c, int32(id))
	if err != nil {
		return nil, err
	}
	if existImage == nil {
		return nil, commonerrors.NewNotFound("get image by id", idStr)
	}
	importImage, err := h.dbClient.GetImportImageByImageID(c, existImage.ID)
	if err != nil {
		return nil, err
	}
	if importImage == nil {
		return nil, commonerrors.NewNotFound("get import image by id", idStr)
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

func (h *ImageHandler) dispatchImportImageJob(c *gin.Context, image *model.Image, info *model.ImageImportJob) (*batchv1.Job, error) {
	jobName := generateImportImageJobName(image.ID)
	imagePullSecrets, err := h.listImagePullSecretsName(c, h.Client, DefaultNamespace)
	if err != nil {
		return nil, err
	}
	job, err := newImportImageJob(image.ID, jobName, SyncerImage, imagePullSecrets, &ImportImageEnv{
		SourceImageName: info.SrcTag,
		DestImageName:   info.DstName,
		OsArch:          fmt.Sprintf(OSArchFormat, info.Os, info.Arch),
		Os:              info.Os,
		Arch:            info.Arch,
	}, image.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err = h.Client.Create(c.Request.Context(), job); err != nil {
		return nil, err
	}
	return job, nil
}

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
						},
					},
					RestartPolicy:    corev1.RestartPolicyNever,
					ImagePullSecrets: make([]corev1.LocalObjectReference, 0),
				},
			},
		},
	}
	for _, secret := range imagePullSecrets {
		job.Spec.Template.Spec.ImagePullSecrets = append(job.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{
			Name: secret,
		})
	}
	volumeName := "registry-auth"
	job.Spec.Template.Spec.Volumes = append(job.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: ImageImportSecretName,
			},
		},
	})
	job.Spec.Template.Spec.Containers[0].VolumeMounts = append(job.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
		Name:      volumeName,
		ReadOnly:  true,
		MountPath: "/root/.docker",
	})
	return job, nil
}

func transEnvMapToEnv(envMap map[string]string) []corev1.EnvVar {
	var envs []corev1.EnvVar
	for k, v := range envMap {
		envs = append(envs, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}
	return envs
}

func defaultSyncImageEnv() map[string]string {
	kvmap := make(map[string]string)
	kvmap[DEBUG] = StringValueTrue
	kvmap[GlobalTLSVerify] = StringValueTrue
	kvmap[OverrideArch] = ""
	kvmap[OverrideOS] = ""
	kvmap[CommandTimeout] = "0"
	kvmap[SourceType] = "docker"
	kvmap[DestinationType] = "docker"
	kvmap[All] = "false"
	kvmap[SrcUserName] = ""
	kvmap[SrcPassword] = ""
	kvmap[TLSVerify] = "true"
	kvmap[DestUserName] = ""
	kvmap[DestPassword] = ""
	kvmap[DestTLSVerify] = "true"

	kvmap[SrcImageEnv] = ""
	kvmap[DestImageEnv] = ""
	kvmap[UpstreamDomain] = ApiServiceName
	return kvmap
}

func generateImportImageJobName(imageId int32) string {
	return fmt.Sprintf("imptimg-%d-%016x", imageId, xxhash.Sum64String(time.Now().String()))
}

func (h *ImageHandler) getImportImageInfo(c context.Context, req *ImportImageServiceRequest) (*ImportImageMetaInfo, error) {
	imageInfo := &ImportImageMetaInfo{
		SourceImageName: req.Source,
		OsArch:          fmt.Sprintf(OSArchFormat, DefaultOS, DefaultArch),
		Os:              DefaultOS,
		Arch:            DefaultArch,
	}
	imageInfo.Status = common.ImageImportingStatus
	if req.SourceRegistry != "" {
		imageInfo.SourceImageName = fmt.Sprintf("%s/%s", req.SourceRegistry, req.Source)
	}
	defaultPushRegistry, err := h.dbClient.GetDefaultRegistryInfo(c)
	if err != nil {
		klog.ErrorS(err, "GetPushRegistryInfo error")
		return nil, commonerrors.NewInternalError("Database Error")
	}
	if defaultPushRegistry == nil {
		return nil, commonerrors.NewBadRequest("Default push registry not exist.Please contract your administrator")
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

func generateTargetImageName(targetRegistryHost, sourceImage string) (string, error) {
	slc := strings.SplitN(sourceImage, "/", 2)
	if len(slc) != 2 {
		return "", fmt.Errorf("source image name is valid")
	}
	repoAndTag := slc[1]
	return fmt.Sprintf("%s/%s/%s", targetRegistryHost, SyncImageProject, repoAndTag), nil
}

func (h *ImageHandler) existImageVlid(c context.Context, destImageName string) (int32, error) {
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

func (h *ImageHandler) checkImageExistsUsingLibrary(ctx context.Context, imageName string, imageInfo *ImportImageMetaInfo) error {
	list := strings.Split(imageInfo.OsArch, "/")
	hostName := strings.Split(imageName, "/")[0]
	os, arch := list[0], list[1]

	sysCtx, err := h.getImageSystemCtx(ctx, hostName, imageName)
	if err != nil {
		klog.Errorf("Error getting system context: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Getting Registry Auth Error: %s", err))
	}

	imageName = fmt.Sprintf("docker://%s", imageName)

	// Parse the reference
	ref, err := alltransports.ParseImageName(imageName)
	if err != nil {
		klog.Errorf("Error parsing reference: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Parsing Reference Error: %s", err))
	}

	// Create an image source
	src, err := ref.NewImageSource(ctx, sysCtx)
	if err != nil {
		klog.Errorf("Image not found or inaccessible: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Image not found or inaccessible: %s", err))
	}
	defer src.Close()

	// Retrieve the manifest to confirm the image exists
	manifest, manifestType, err := src.GetManifest(ctx, nil)
	if err != nil {
		klog.Errorf("Getting Manifest Error: %s", err)
		return commonerrors.NewInternalError(fmt.Sprintf("Getting Manifest Error: %s", err))
	}

	var totalSize int64
	if manifestType == imagespecv1.MediaTypeImageIndex || manifestType == manifestv5.DockerV2ListMediaType {
		var targetDigest imagedigest.Digest
		totalSize += int64(len(manifest))

		if manifestType == imagespecv1.MediaTypeImageIndex {
			var index imagespecv1.Index
			if err := json.Unmarshal(manifest, &index); err != nil {
				klog.Errorf("Error parsing OCI index: %s", err)
				return commonerrors.NewInternalError(fmt.Sprintf("Parsing OCI index Error: %s", err))
			}

			for _, m := range index.Manifests {
				if m.Platform != nil &&
					m.Platform.OS == os &&
					m.Platform.Architecture == arch {
					targetDigest = m.Digest
					break
				}
			}
		} else {
			var schema2List manifestv5.Schema2List
			if err := json.Unmarshal(manifest, &schema2List); err != nil {
				klog.Errorf("Error parsing Docker manifest list: %s", err)
				return commonerrors.NewInternalError(fmt.Sprintf("Parsing Docker manifest list Error: %s", err))
			}

			for _, m := range schema2List.Manifests {
				if m.Platform.OS == os &&
					m.Platform.Architecture == arch {
					targetDigest = m.Digest
					break
				}
			}
		}
		if targetDigest == "" {
			klog.Errorf("No matching manifest found for OS %s and architecture %s", os, arch)
			return commonerrors.NewInternalError(fmt.Sprintf("No matching manifest found for OS %s and architecture %s", os, arch))
		}

		manifest, manifestType, err = src.GetManifest(ctx, &targetDigest)
		if err != nil {
			klog.Errorf("Error getting platform-specific manifest: %s", err)
			return commonerrors.NewInternalError(fmt.Sprintf("Getting platform-specific manifest Error: %s", err))
		}
	}

	totalSize += int64(len(manifest))
	switch manifestType {
	case imagespecv1.MediaTypeImageManifest:
		var v1Manifest imagespecv1.Manifest
		if err := json.Unmarshal(manifest, &v1Manifest); err != nil {
			klog.Errorf("Error parsing OCI manifest: %s", err)
			return commonerrors.NewInternalError(fmt.Sprintf("Parsing OCI manifest Error: %s", err))
		}
		for _, layer := range v1Manifest.Layers {
			totalSize += layer.Size
		}
		totalSize += v1Manifest.Config.Size

	case manifestv5.DockerV2Schema2MediaType:
		var v2Manifest manifestv5.Schema2
		if err := json.Unmarshal(manifest, &v2Manifest); err != nil {
			klog.Errorf("Error parsing Docker manifest: %s", err)
			return commonerrors.NewInternalError(fmt.Sprintf("Parsing Docker manifest Error: %s", err))
		}
		for _, layer := range v2Manifest.LayerInfos() {
			totalSize += layer.Size
		}
		totalSize += v2Manifest.ConfigInfo().Size
	}

	imageInfo.Size = totalSize

	klog.Infof("Image %s exists, size: %d", imageName, totalSize)
	return nil
}

func (h *ImageHandler) getImageSystemCtx(ctx context.Context, hostName string, imageName string) (*v5types.SystemContext, error) {
	sysCtx := &v5types.SystemContext{DockerInsecureSkipTLSVerify: v5types.OptionalBoolTrue}
	if strings.HasSuffix(hostName, "docker.io") {

		// library/alpine:latest
		item := strings.Join(strings.Split(imageName, "/")[1:], "/")
		// library/alpine:latest -> library/alpine
		imagePath := strings.Split(item, ":")[0]

		token, err := h.fetchDockerToken(ctx, imagePath)
		if err != nil {
			klog.Errorf("Error fetching token, err is: %s \n", err)
			return nil, err
		}
		sysCtx.DockerBearerRegistryToken = token
	} else {
		accountInfo, err := h.dbClient.GetRegistryInfoByUrl(ctx, hostName)
		if err != nil {
			klog.Errorf("Error getting registry info, err is: %s \n", err)
			return nil, err
		}

		if accountInfo != nil {
			password := ""
			if accountInfo.Password != "" {
				password, err = crypto.NewCrypto().Decrypt(accountInfo.Password)
				if err != nil {
					klog.Errorf("Error decrypting password, err is: %s \n", err)
					return nil, err
				}
			}

			userName := ""
			if accountInfo.Username != "" {
				userName, err = crypto.NewCrypto().Decrypt(accountInfo.Username)
				if err != nil {
					klog.Errorf("Error decrypting username, err is: %s \n", err)
					return nil, err
				}
			}
			sysCtx.DockerAuthConfig = &v5types.DockerAuthConfig{
				Username: userName,
				Password: password,
			}
		}
	}
	return sysCtx, nil
}

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
		param := v1.CvtStringToParam(p)
		if param != nil {
			result = append(result, *param)
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
	
	// Filter by workload ID if specified (using JSONB containment query)
	if query.Workload != "" {
		dbSql = append(dbSql, sqrl.Expr("inputs::jsonb @> ?", 
			fmt.Sprintf(`[{"name":"workload","value":"%s"}]`, query.Workload)))
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

// convertOpsJobToExportedImages converts ops_job records to ExportedImageItem slice.
func convertOpsJobToExportedImages(jobs []*dbClient.OpsJob) []ExportedImageItem {
	result := make([]ExportedImageItem, 0, len(jobs))
	
	for _, job := range jobs {
		item := ExportedImageItem{
			JobId:       job.JobId,
			Status:      dbutils.ParseNullString(job.Phase),
			UserName:    dbutils.ParseNullString(job.UserName),
			CreatedTime: dbutils.ParseNullTime(job.CreationTime),
			StartTime:   dbutils.ParseNullTime(job.StartTime),
			EndTime:     dbutils.ParseNullTime(job.EndTime),
		}
		
		// Parse inputs to extract source image
		if len(job.Inputs) > 0 {
			inputsStr := string(job.Inputs)
			// Parse TEXT[] format: {"{name:workload,value:xxx}","{name:image,value:yyy}"}
			if strings.Contains(inputsStr, "name:image") {
				parts := strings.Split(inputsStr, ",")
				for _, part := range parts {
					if strings.Contains(part, "name:image") && strings.Contains(part, "value:") {
						start := strings.Index(part, "value:") + 6
						end := len(part)
						if idx := strings.Index(part[start:], "}"); idx != -1 {
							end = start + idx
						}
						item.SourceImage = strings.TrimSpace(part[start:end])
						break
					}
				}
			}
		}
		
		// Parse outputs to extract target image
		if outputsStr := dbutils.ParseNullString(job.Outputs); outputsStr != "" {
			var outputs []v1.Parameter
			if err := json.Unmarshal([]byte(outputsStr), &outputs); err == nil {
				for _, param := range outputs {
					switch param.Name {
					case "target":
						item.TargetImage = param.Value
					case "message":
						item.Message = param.Value
					}
				}
			}
		}
		
		// Parse conditions to extract failure message if needed
		if item.Message == "" {
			if conditionsStr := dbutils.ParseNullString(job.Conditions); conditionsStr != "" {
				var conditions []metav1.Condition
				if err := json.Unmarshal([]byte(conditionsStr), &conditions); err == nil {
					for i := len(conditions) - 1; i >= 0; i-- {
						if conditions[i].Message != "" {
							item.Message = conditions[i].Message
							break
						}
					}
				}
			}
		}
		
		result = append(result, item)
	}
	
	return result
}

// cvtExportedImagesToResponse converts ExportedImageItem slice to GetImageResponse format (grouped by repo).
func cvtExportedImagesToResponse(exportedImages []ExportedImageItem) []GetImageResponseItem {
	repoMap := map[string]int{}
	result := make([]GetImageResponseItem, 0)
	
	// Note: Target image from controller doesn't include registry host
	// It's in format: "Custom/namespace/repository:tag"
	// We need to get the default registry to construct full path
	
	for _, item := range exportedImages {
		// Skip if no target image
		if item.TargetImage == "" {
			continue
		}
		
		// Parse target image format: "Custom/rocm/pytorch:20250112"
		// Format: [project]/[namespace]/[repository]:[tag]
		// Example: "Custom" is the Harbor project name
		var repo, imageTag string
		
		// Split by last ":"
		imageWithoutTag := item.TargetImage
		if colonIdx := strings.LastIndex(item.TargetImage, ":"); colonIdx != -1 {
			imageWithoutTag = item.TargetImage[:colonIdx]
			imageTag = item.TargetImage[colonIdx+1:]
		} else {
			imageTag = "latest"
		}
		
		// repo is everything after first "/" (Custom/rocm/pytorch → rocm/pytorch)
		if slashIdx := strings.Index(imageWithoutTag, "/"); slashIdx != -1 {
			repo = imageWithoutTag[slashIdx+1:] // "rocm/pytorch"
		} else {
			repo = imageWithoutTag
		}
		
		// Create artifact item
		artifact := ArtifactItem{
			ImageTag:    imageTag,
			Description: fmt.Sprintf("Exported from source: %s", item.SourceImage),
			CreatedTime: timeutil.FormatRFC3339(item.CreatedTime),
			UserName:    item.UserName,
			Status:      item.Status,
			IncludeType: "custom", // Custom user image exported from workload
			// Note: Size, Arch, Os, Digest would need to be fetched from Harbor API
		}
		
		// For exported images, we use a placeholder registry host
		// In production, this should be fetched from the default registry config
		registryHost := "harbor.exported" // Placeholder, could be fetched from config
		
		// Group by registry+repo
		fullUrl := strings.Join([]string{registryHost, repo}, "/")
		if index, ok := repoMap[fullUrl]; !ok {
			// New repo
			result = append(result, GetImageResponseItem{
				RegistryHost: registryHost,
				Repo:         repo,
				Artifacts:    []ArtifactItem{artifact},
			})
			repoMap[fullUrl] = len(result) - 1
		} else {
			// Existing repo, append artifact
			result[index].Artifacts = append(result[index].Artifacts, artifact)
		}
	}
	
	return result
}
