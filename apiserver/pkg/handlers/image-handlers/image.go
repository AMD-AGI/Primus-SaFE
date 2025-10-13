package image_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/crypto"
	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
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
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *ImageHandler) createImage(c *gin.Context) (interface{}, error) {
	req := &CreateImageRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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
	})
	if err != nil {
		klog.ErrorS(err, "fail to SelectImages", "sql")
		return nil, err
	}

	results := &GetImageResponse{
		TotalCount: count,
	}

	results.Items = cvtImageToResponse(images, DefaultOS, DefaultArch)
	return results, nil
}

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
	return query, nil
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
			CreatedTime: image.CreatedAt.Format(time.DateTime),
			UserName:    image.CreatedBy,
			Status:      image.Status,
			Id:          int32(image.ID),
			IncludeType: image.Source, // 1: sync, 2: input
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
	var resp = &ImportImageResponse{}

	body := &ImportImageServiceRequest{}
	if err = c.ShouldBindJSON(&body); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}

	uid := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	importImageJobName := generateImportImageJobName(uid)

	imageInfo, err := h.getImportImageInfo(c, body)
	if err != nil {
		return nil, err
	}

	// 检查 image 是否已存在
	existImageID, err := h.existImageVlid(c, imageInfo.DestImageName)
	if err != nil {
		return nil, err
	}
	if existImageID != 0 {
		resp.AlreadyImageID = existImageID
		resp.Message = "Image already existed. We don't need to import it again"
		return resp, nil
	}

	imagePullSecrets, err := h.listImagePullSecretsName(c, h.Client, DefaultNamespace)
	if err != nil {
		return nil, err
	}

	var importImageEnv = &ImportImageEnv{
		SourceImageName: imageInfo.SourceImageName,
		DestImageName:   imageInfo.DestImageName,
		OsArch:          imageInfo.OsArch,
		Description:     fmt.Sprintf("Import from %s", imageInfo.SourceImageName),
	}

	var relationDigest = map[string]interface{}{}

	defaultDigestItem := &RelationDigest{
		Digest: "",             // 录入的镜像为空
		Size:   imageInfo.Size, // 录入的时候会获取镜像大小，同步则是在同步过程中再获取
	}
	relationDigest[importImageEnv.OsArch] = defaultDigestItem
	dbImage := &model.Image{
		Tag:            imageInfo.DestImageName,
		CreatedBy:      userName,
		CreatedAt:      time.Now().UTC(),
		Description:    fmt.Sprintf("Import from %s", importImageEnv.SourceImageName),
		Status:         common.ImageImportingStatus,
		RelationDigest: relationDigest,
		Source:         "import",
	}
	if err := h.dbClient.UpsertImage(c, dbImage); err != nil {
		return nil, err
	}

	var job *batchv1.Job
	job, err = newImportImageJob(dbImage.ID, importImageJobName, SyncerImage, imagePullSecrets, uid, importImageEnv, userName)
	if err != nil {
		return nil, err
	}
	if err = h.Client.Create(c.Request.Context(), job); err != nil {
		return nil, err
	}
	importImageInfo := &model.ImageImportJob{
		SrcTag:    imageInfo.SourceImageName,
		DstName:   imageInfo.DestImageName,
		Os:        importImageEnv.Os,
		Arch:      importImageEnv.Arch,
		JobName:   job.Name,
		CreatedAt: time.Now().UTC(),
		ImageID:   dbImage.ID,
	}

	if err := h.dbClient.UpsertImageImportJob(c, importImageInfo); err != nil {
		return nil, err
	}
	return resp, nil
}

func newImportImageJob(
	imageId int32,
	jobName,
	syncImage string,
	imagePullSecrets []string,
	uid string,
	env *ImportImageEnv,
	userName string) (*batchv1.Job, error) {
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

	// 传递时必填
	kvmap[SrcImageEnv] = ""
	kvmap[DestImageEnv] = ""
	kvmap[UpstreamDomain] = ApiServiceName
	return kvmap
}

func generateImportImageJobName(uid string) string {
	return fmt.Sprintf("imptimg-%s-%016x", uid, xxhash.Sum64String(time.Now().String()))
}

// getFullImportName 获取完整的 image name
func (h *ImageHandler) getImportImageInfo(c context.Context, req *ImportImageServiceRequest) (*ImportImageMetaInfo, error) {
	var imageInfo = &ImportImageMetaInfo{
		SourceImageName: req.Source,
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

func (h *ImageHandler) checkImageExistsUsingLibrary(ctx context.Context, imageName string, imageInfo *ImportImageMetaInfo) bool {
	list := strings.Split(imageInfo.OsArch, "/")
	hostName := strings.Split(imageName, "/")[0]
	var os, arch = list[0], list[1]

	sysCtx, err := h.getImageSystemCtx(ctx, hostName, imageName)
	if err != nil {
		klog.Errorf("Error getting system context: %s", err)
		return false
	}

	// 构造镜像引用
	imageName = fmt.Sprintf("docker://%s", imageName)

	// Parse the reference
	ref, err := alltransports.ParseImageName(imageName)
	if err != nil {
		klog.Errorf("Error parsing reference: %s", err)
		return false
	}

	// Create an image source
	src, err := ref.NewImageSource(ctx, sysCtx)
	if err != nil {
		klog.Errorf("Image not found or inaccessible: %s", err)
		return false
	}
	defer src.Close()

	// Retrieve the manifest to confirm the image exists
	manifest, manifestType, err := src.GetManifest(ctx, nil)
	if err != nil {
		klog.Errorf("Image not found or inaccessible: %s", err)
		return false
	}

	var totalSize int64
	// 处理manifest list的情况
	if manifestType == imagespecv1.MediaTypeImageIndex || manifestType == manifestv5.DockerV2ListMediaType {
		// 解析manifest list
		var targetDigest imagedigest.Digest
		// manifest list 大小进行累加
		totalSize += int64(len(manifest))

		if manifestType == imagespecv1.MediaTypeImageIndex {
			// 处理OCI格式的manifest list
			var index imagespecv1.Index
			if err := json.Unmarshal(manifest, &index); err != nil {
				klog.Errorf("Error parsing OCI index: %s", err)
				return false
			}

			// 查找匹配的manifest
			for _, m := range index.Manifests {
				if m.Platform != nil &&
					m.Platform.OS == os &&
					m.Platform.Architecture == arch {
					targetDigest = m.Digest
					break
				}
			}
		} else {
			// 处理Docker格式的manifest list
			var schema2List manifestv5.Schema2List
			if err := json.Unmarshal(manifest, &schema2List); err != nil {
				klog.Errorf("Error parsing Docker manifest list: %s", err)
				return false
			}

			// 查找匹配的manifest
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
			return false
		}

		// 获取特定平台的manifest
		manifest, manifestType, err = src.GetManifest(ctx, &targetDigest)
		if err != nil {
			klog.Errorf("Error getting platform-specific manifest: %s", err)
			return false
		}
	}

	// manifest 大小进行累加
	totalSize += int64(len(manifest))
	// 根据manifest类型解析大小信息
	switch manifestType {
	case imagespecv1.MediaTypeImageManifest:
		// OCI格式镜像
		var v1Manifest imagespecv1.Manifest
		if err := json.Unmarshal(manifest, &v1Manifest); err != nil {
			klog.Errorf("Error parsing OCI manifest: %s", err)
			return false
		}
		// 计算所有层的大小总和
		for _, layer := range v1Manifest.Layers {
			totalSize += layer.Size
		}
		// 加上配置文件大小
		totalSize += v1Manifest.Config.Size

	case manifestv5.DockerV2Schema2MediaType:
		// Docker格式镜像
		var v2Manifest manifestv5.Schema2
		if err := json.Unmarshal(manifest, &v2Manifest); err != nil {
			klog.Errorf("Error parsing Docker manifest: %s", err)
			return false
		}
		// 计算所有层的大小总和
		for _, layer := range v2Manifest.LayerInfos() {
			totalSize += layer.Size
		}
		// 加上配置文件大小
		totalSize += v2Manifest.ConfigInfo().Size
	}

	imageInfo.Size = totalSize

	klog.Infof("Image %s exists", imageName)
	return true
}

func (h *ImageHandler) getImageSystemCtx(ctx context.Context, hostName string, imageName string) (*v5types.SystemContext, error) {
	// 创建系统上下文
	var sysCtx = &v5types.SystemContext{DockerInsecureSkipTLSVerify: v5types.OptionalBoolTrue}
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
		// 获取账号信息，如果有的话
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

func (h *ImageHandler) fetchDockerToken(ctx context.Context, imagePath string) (string, error) {
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
