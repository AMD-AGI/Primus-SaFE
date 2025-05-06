/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package s3

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

var (
	clusters = map[string]*S3Cluster{}
)

type S3Cluster struct {
	clients     map[string]Interface
	clusterConf *ClusterConfig
	lock        sync.Mutex
}

func (s *S3Cluster) GetOrInitBucketClient(ctx context.Context, bucket string) (Interface, error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if client, ok := s.clients[bucket]; ok {
		return client, nil
	}
	client, err := newClient(ctx, s.clusterConf, bucket)
	if err != nil {
		return nil, err
	}
	s.clients[bucket] = client
	return client, nil
}

type S3Client struct {
	s3Config   *aws.Config
	conf       *ClusterConfig
	bucketName string
}

func InitS3(ctx context.Context) error {
	err := initConfigs(ctx)
	if err != nil {
		klog.ErrorS(err, "fail to init s3 config")
		return err
	}
	if !conf.Enabled() {
		return nil
	}
	k8sClient, _, err := k8sclient.NewClientSetInCluster()
	if err != nil {
		klog.ErrorS(err, "fail to new clientSet in cluster")
		return errors.NewError().WithError(err).WithMessage("fail to new clientSet in cluster")
	}
	for i, cluster := range conf.Clusters {
		clusterConfig, err := loadCLusterConfig(ctx, k8sClient, &cluster)
		if err != nil {
			klog.ErrorS(err, fmt.Sprintf("fail to load cluster config. %d of %d", i+1, len(conf.Clusters)))
			return err
		}
		s3Cluster := &S3Cluster{
			clients:     map[string]Interface{},
			clusterConf: &clusterConfig,
		}
		for _, module := range cluster.Module {
			clusters[module] = s3Cluster
		}
	}
	return nil
}

func GetS3Token(ctx context.Context, module string) (string, error) {
	moduleClient, ok := clusters[module]
	if !ok {
		return "", errors.NewError().WithMessage("module not found")
	}
	ak := moduleClient.clusterConf.AccessKey
	sk := moduleClient.clusterConf.SecretKey
	s := fmt.Sprintf("d%02d%sd%sd", len(ak), ak, sk)
	return base64.StdEncoding.EncodeToString([]byte(s)), nil
}

func Instance(module, bucket string) (Interface, error) {
	if moduleClients, ok := clusters[module]; !ok {
		klog.Errorf("module not found, clusters: %+v\n", clusters)
		return nil, errors.NewError().WithMessage("module not found")
	} else {
		if client, ok := moduleClients.clients[bucket]; ok {
			return client, nil
		} else {
			return moduleClients.GetOrInitBucketClient(context.Background(), bucket)
		}
	}
}

func getS3Secret(ctx context.Context, k8sClient kubernetes.Interface, c *SingleClusterConfig) (ak, sk string, err error) {
	secret, err := k8sClient.CoreV1().Secrets(c.Namespace).Get(
		ctx, c.Secret, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "fail to get secret")
		return
	}
	data, ok := secret.Data["AccessKey"]
	if !ok {
		klog.Errorf("fail to find AccessKey of secret")
		return
	}
	ak = string(data)

	data, ok = secret.Data["SecretKey"]
	if !ok {
		klog.Errorf("fail to find SecretKey of secret")
		return
	}
	sk = string(data)
	return
}

func newClient(ctx context.Context, conf *ClusterConfig, bucketName string) (Interface, error) {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(conf.AccessKey, conf.SecretKey, conf.Token),
		Endpoint:         &conf.Endpoint,
		Region:           &conf.BucketRegion,
		S3ForcePathStyle: aws.Bool(true),
	}
	cli := &S3Client{
		s3Config:   s3Config,
		bucketName: bucketName,
	}
	klog.Info("endpoint:", s3Config.Endpoint)
	if ok, err := cli.IsBucketExisted(); err != nil {
		return nil, err
	} else if !ok {
		err = cli.CreateBucket()
		if err != nil {
			klog.ErrorS(err, "fail to create bucket")
		}
		return nil, err
	}
	klog.Infof("init s3 client successfully!. endpoint: %+v", s3Config.Endpoint)
	return cli, nil

}

func (c *S3Client) GetS3Config() ClusterConfig {
	return *c.conf
}

func (c *S3Client) CreateBucket() error {
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}
	klog.Info("start to create bucket:", c.bucketName)

	bucket := aws.String(c.bucketName)

	timeout := int64(commonconfig.GetS3Timeout())
	if timeout == 0 {
		_, err = s3Client.CreateBucket(&s3.CreateBucketInput{Bucket: bucket})
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(timeout))
		defer cancel()
		_, err = s3Client.CreateBucketWithContext(ctx, &s3.CreateBucketInput{Bucket: bucket})
	}
	if err != nil {
		return err
	}
	if err = c.putBucketLifecycle(s3Client); err != nil {
		return err
	}
	if err = c.putBucketPolicy(s3Client); err != nil {
		return err
	}
	klog.Infof(" created bucket %s Successfully", c.bucketName)
	return nil
}

func (c *S3Client) putBucketLifecycle(s3Client *s3.S3) error {
	_, err := s3Client.PutBucketLifecycleConfiguration(&s3.PutBucketLifecycleConfigurationInput{
		Bucket: aws.String(c.bucketName),
		LifecycleConfiguration: &s3.BucketLifecycleConfiguration{
			Rules: []*s3.LifecycleRule{
				{
					ID:     aws.String("ExpireAfterOneDay"),
					Filter: &s3.LifecycleRuleFilter{Prefix: aws.String("")},
					Expiration: &s3.LifecycleExpiration{
						Days: aws.Int64(int64(commonconfig.GetS3ExpireDays())),
					},
					Status: aws.String("Enabled"),
				},
			},
		},
	})
	if err != nil {
		klog.ErrorS(err, "fail to PutBucketLifecycle", "bucket", c.bucketName)
		return err
	}
	return nil
}

func (c *S3Client) putBucketPolicy(s3Client *s3.S3) error {
	policy := `{
        "Statement": [
            {
                "Sid": "PublicReadGetObject",
                "Effect": "Allow", 
                "Principal": "*",
                "Action": "s3:GetObject",
                "Resource": "arn:aws:s3:::` + c.bucketName + `/*"
            }
        ]
    }`
	_, err := s3Client.PutBucketPolicy(&s3.PutBucketPolicyInput{
		Bucket: aws.String(c.bucketName),
		Policy: aws.String(policy),
	})
	if err != nil {
		klog.ErrorS(err, "fail to PutBucketPolicy", "bucket", c.bucketName)
		return err
	}
	return nil
}

func (c *S3Client) BeforeMultipart(ctx context.Context, key string, timeout int64) (*s3.S3, string, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return nil, "", err
	}
	var initReq *s3.CreateMultipartUploadOutput
	input := &s3.CreateMultipartUploadInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	}
	if timeout == 0 {
		timeout = int64(commonconfig.GetS3Timeout())
	}
	if timeout == 0 {
		initReq, err = s3Client.CreateMultipartUpload(input)
	} else {
		ctx2, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
		defer cancel()
		initReq, err = s3Client.CreateMultipartUploadWithContext(ctx2, input)
	}
	if err != nil {
		return nil, "", err
	}
	return s3Client, *initReq.UploadId, nil
}

func (c *S3Client) MultipartUpload(ctx context.Context, s3Client *s3.S3,
	key, uploadId, value string, partNumber, timeout int64) (*s3.CompletedPart, error) {
	partInput := &s3.UploadPartInput{
		Bucket:     aws.String(c.bucketName),
		Key:        aws.String(key),
		UploadId:   aws.String(uploadId),
		PartNumber: aws.Int64(partNumber),
		Body:       bytes.NewReader([]byte(value)),
	}
	var err error
	var partResp *s3.UploadPartOutput
	if timeout == 0 {
		timeout = int64(commonconfig.GetS3Timeout())
	}
	if timeout == 0 {
		partResp, err = s3Client.UploadPart(partInput)
	} else {
		ctx2, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
		defer cancel()
		partResp, err = s3Client.UploadPartWithContext(ctx2, partInput)
	}

	if err != nil {
		return nil, err
	}
	klog.Infof("multi part upload, key: %s, partNumber: %d", key, partNumber)
	return &s3.CompletedPart{
		ETag:       partResp.ETag,
		PartNumber: aws.Int64(partNumber),
	}, nil
}

func (c *S3Client) AfterMultipart(ctx context.Context, s3Client *s3.S3,
	key, uploadId string, uploadedParts []*s3.CompletedPart, timeout int64) error {
	if len(uploadedParts) == 0 {
		return nil
	}
	input := &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(c.bucketName),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadId),
		MultipartUpload: &s3.CompletedMultipartUpload{Parts: uploadedParts},
	}
	if timeout == 0 {
		timeout = int64(commonconfig.GetS3Timeout())
	}
	var err error
	if timeout == 0 {
		ctx2, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
		defer cancel()
		_, err = s3Client.CompleteMultipartUploadWithContext(ctx2, input)
	} else {
		_, err = s3Client.CompleteMultipartUpload(input)
	}
	if err != nil {
		klog.ErrorS(err, "fail to complete multipart upload")
		return err
	}
	return nil
}

func (c *S3Client) AbortMultipartUpload(s3Client *s3.S3, key, uploadId string) error {
	_, err := s3Client.AbortMultipartUpload(&s3.AbortMultipartUploadInput{
		Bucket:   aws.String(c.bucketName),
		Key:      aws.String(key),
		UploadId: aws.String(uploadId),
	})
	if err != nil {
		klog.ErrorS(err, "fail to abort multipart upload")
		return err
	}
	return nil
}

func (c *S3Client) IsBucketExisted() (bool, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return false, err
	}
	input := &s3.HeadBucketInput{
		Bucket: aws.String(c.bucketName),
	}

	timeout := int64(commonconfig.GetS3Timeout())
	if timeout == 0 {
		_, err = s3Client.HeadBucket(input)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(timeout))
		defer cancel()
		_, err = s3Client.HeadBucketWithContext(ctx, input)
	}
	if err != nil {
		return false, nil
	}
	klog.Infof("the bucket %s already exists", c.bucketName)
	return true, nil
}

func (c *S3Client) ListBuckets() (*s3.ListBucketsOutput, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return nil, err
	}

	result, err := s3Client.ListBuckets(nil)
	if err != nil {
		klog.ErrorS(err, "fail to list buckets: %s", err)
		return nil, err
	}
	return result, nil
}

func (c *S3Client) DeleteBucket() error {
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}
	output, err := s3Client.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(c.bucketName),
	})
	if err != nil {
		return err
	}
	klog.Infof("delete bucket: %s, output: %s", c.bucketName, output.String())
	return nil
}

func (c *S3Client) PutObject(ctx context.Context, key, value string, timeout int64) error {
	if key == "" || value == "" {
		return fmt.Errorf("the object key or value is empty")
	}
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}
	if timeout == 0 {
		timeout = int64(commonconfig.GetS3Timeout())
	}
	if timeout == 0 {
		_, err = s3Client.PutObject(&s3.PutObjectInput{
			Bucket: aws.String(c.bucketName),
			Key:    aws.String(key),
			Body:   bytes.NewReader([]byte(value)),
		})
	} else {
		ctx2, cancel := context.WithTimeout(ctx, time.Second*time.Duration(timeout))
		defer cancel()
		_, err = s3Client.PutObjectWithContext(ctx2, &s3.PutObjectInput{
			Bucket: aws.String(c.bucketName),
			Key:    aws.String(key),
			Body:   bytes.NewReader([]byte(value)),
		})
	}

	if err != nil {
		klog.ErrorS(err, "fail to upload %q to %q", key, c.bucketName)
		return err
	}
	klog.Infof(" uploaded %q to %q Successfully", key, c.bucketName)
	return nil
}

func (c *S3Client) PutObjectWithContext(ctx aws.Context, key string, body io.ReadSeeker) (*s3.PutObjectOutput, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return nil, err
	}

	return s3Client.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
		Body:   body,
	})
}

func (c *S3Client) Upload(key string, body io.ReadSeeker) (*s3manager.UploadOutput, error) {
	sess, err := session.NewSession(c.s3Config)
	if err != nil {
		klog.Errorf("fail to create session: %s", err)
		return nil, err
	}
	uploader := s3manager.NewUploader(sess)
	return uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
		Body:   body,
	})

}

func (c *S3Client) ListObject() (*s3.ListObjectsOutput, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return nil, err
	}

	output, err := s3Client.ListObjects(&s3.ListObjectsInput{
		Bucket: aws.String(c.bucketName),
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func (c *S3Client) StreamDownloadFile(key string) (io.ReadCloser, error) {
	resp, err := c.GetObjectResp(key)
	if err != nil {
		klog.ErrorS(err, "fail to get object %s from bucket: %s", key, c.bucketName)
		return nil, err
	}
	return resp.Body, nil
}

func (c *S3Client) GetObjectResp(key string) (*s3.GetObjectOutput, error) {
	if key == "" {
		return nil, fmt.Errorf("the object key is empty")
	}
	s3Client, err := c.newClient()
	if err != nil {
		return nil, err
	}
	return s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
}

func (c *S3Client) GetObject(key string) error {
	resp, err := c.GetObjectResp(key)
	if err != nil {
		klog.ErrorS(err, "fail to get object %s from bucket: %s", key, c.bucketName)
		return err
	}

	defer resp.Body.Close()

	// Write object content to a file
	file, err := os.Create(key)
	if err != nil {
		klog.ErrorS(err, "failed to create file")
		return err
	}
	defer file.Close()

	_, err = file.ReadFrom(resp.Body)
	if err != nil {
		klog.Errorf("failed to write object content to file, %v", err)
	}
	klog.Infof("Object %s downloaded successfully", key)
	return nil
}

func (c *S3Client) GetObjectStream(key string) (*s3.GetObjectOutput, error) {
	if key == "" {
		return nil, fmt.Errorf("the object key is empty")
	}
	s3Client, err := c.newClient()
	if err != nil {
		return nil, err
	}
	resp, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	return resp, err
}

func (c *S3Client) DeleteObject(key string) error {
	if key == "" {
		return fmt.Errorf("the object key is empty")
	}
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}

	output, err := s3Client.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	klog.Infof("delete object: %s, output: %s", key, output.String())
	return nil
}

func (c *S3Client) newClient() (*s3.S3, error) {
	newSession, err := session.NewSession(c.s3Config)
	if err != nil {
		klog.Errorf("fail to create session: %s", err)
		return nil, err
	}

	s3Client := s3.New(newSession)
	return s3Client, nil
}

func NewS3ClientWithConfig(ctx context.Context, s3Config *aws.Config, bucketName string) (*S3Client, error) {
	cli := &S3Client{
		s3Config:   s3Config,
		bucketName: bucketName,
	}
	return cli, nil
}

func (c *S3Client) PresignGetObject(ctx context.Context, key string, expire time.Duration) (string, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return "", err
	}

	req, _ := s3Client.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})

	url, err := req.Presign(expire)
	if err != nil {
		klog.ErrorS(err, "fail to presign get object", "key", key)
		return "", err
	}

	klog.Infof("generated presigned URL for GetObject: %s", url)
	return url, nil
}

func (c *S3Client) PresignPutObject(ctx context.Context, key string, expire time.Duration) (string, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return "", err
	}

	req, _ := s3Client.PutObjectRequest(&s3.PutObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})

	url, err := req.Presign(expire)
	if err != nil {
		klog.ErrorS(err, "fail to presign put object", "key", key)
		return "", err
	}

	klog.Infof("generated presigned URL for PutObject: %s", url)
	return url, nil
}

func (c *S3Client) PresignMultipartUpload(ctx context.Context, key string, partSize int64, expire time.Duration) (string, []string, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return "", nil, err
	}

	initReq, err := s3Client.CreateMultipartUpload(&s3.CreateMultipartUploadInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	})
	if err != nil {
		klog.ErrorS(err, "fail to initiate multipart upload")
		return "", []string{}, err
	}

	uploadId := *initReq.UploadId
	partUrls := []string{}
	partNumber := 1
	for partStart := int64(0); partStart < partSize; partStart += partSize {
		partEnd := partStart + partSize
		if partEnd > partSize {
			partEnd = partSize
		}

		req, _ := s3Client.UploadPartRequest(&s3.UploadPartInput{
			Bucket:     aws.String(c.bucketName),
			Key:        aws.String(key),
			UploadId:   aws.String(uploadId),
			PartNumber: aws.Int64(int64(partNumber)),
		})

		url, err := req.Presign(expire)
		if err != nil {
			klog.ErrorS(err, "fail to presign multipart upload part", "key", key, "partNumber", partNumber)
			return "", nil, err
		}

		partUrls = append(partUrls, url)
		partNumber++
	}

	klog.Infof("Generated presigned URLs for multipart upload")
	return uploadId, partUrls, nil
}

func (c *S3Client) PresignCompleteMultipartUpload(ctx context.Context, key, uploadId string, parts []*s3.CompletedPart, expire time.Duration) (string, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return "", err
	}

	req, _ := s3Client.CompleteMultipartUploadRequest(&s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(c.bucketName),
		Key:             aws.String(key),
		UploadId:        aws.String(uploadId),
		MultipartUpload: &s3.CompletedMultipartUpload{Parts: parts},
	})

	url, err := req.Presign(expire)
	if err != nil {
		klog.ErrorS(err, "fail to presign complete multipart upload", "key", key, "uploadId", uploadId)
		return "", err
	}

	klog.Infof("generated presigned URL for CompleteMultipartUpload: %s", url)
	return url, nil
}

func (c *S3Client) PresignListObjects(ctx context.Context, prefix string, maxKeys int64, expire time.Duration) (string, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return "", err
	}

	req, _ := s3Client.ListObjectsV2Request(&s3.ListObjectsV2Input{
		Bucket:    aws.String(c.bucketName),
		Prefix:    aws.String(prefix),
		MaxKeys:   aws.Int64(maxKeys),
		Delimiter: aws.String("/"),
	})

	url, err := req.Presign(expire)
	if err != nil {
		klog.ErrorS(err, "failed to presign ListObjectsV2", "prefix", prefix)
		return "", err
	}

	klog.Infof("generated presigned URL for ListObjectsV2: %s", url)
	return url, nil
}

func (c *S3Client) MoveObject(ctx context.Context, sourceKey string, destinationKey string) error {
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}

	copyObjectInput := &s3.CopyObjectInput{
		Bucket:     &c.bucketName,
		Key:        aws.String(destinationKey),
		CopySource: aws.String(c.bucketName + "/" + sourceKey),
	}
	_, err = s3Client.CopyObject(copyObjectInput)
	if err != nil {
		klog.Error("failed to copy object:", err)
		return err
	}

	err = c.DeleteObject(sourceKey)
	if err != nil {
		klog.Error("failed to delete original object:", err)
		return err
	}
	return nil
}

func (c *S3Client) CopyObject(ctx context.Context, sourceKey string, destinationKey string) error {
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}

	copyObjectInput := &s3.CopyObjectInput{
		Bucket:     &c.bucketName,
		Key:        aws.String(destinationKey),
		CopySource: aws.String(c.bucketName + "/" + sourceKey),
	}
	_, err = s3Client.CopyObject(copyObjectInput)
	if err != nil {
		klog.Error("failed to copy object:", err)
		return err
	}

	klog.Infof("copied object %s to %s", sourceKey, destinationKey)
	return nil
}

func (c *S3Client) CopyDirectory(ctx context.Context, sourcePrefix string, destinationPrefix string) error {
	s3Client, err := c.newClient()
	if err != nil {
		return err
	}

	if !strings.HasSuffix(sourcePrefix, "/") {
		sourcePrefix += "/"
	}
	if !strings.HasSuffix(destinationPrefix, "/") {
		destinationPrefix += "/"
	}

	listObjectsInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(c.bucketName),
		Prefix: aws.String(sourcePrefix),
	}

	var copyErrors []error

	err = s3Client.ListObjectsV2PagesWithContext(ctx, listObjectsInput, func(output *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, object := range output.Contents {
			sourceKey := *object.Key
			relativeKey := strings.TrimPrefix(sourceKey, sourcePrefix)
			destinationKey := destinationPrefix + relativeKey

			copyObjectInput := &s3.CopyObjectInput{
				Bucket:     &c.bucketName,
				Key:        aws.String(destinationKey),
				CopySource: aws.String(c.bucketName + "/" + sourceKey),
			}
			_, err = s3Client.CopyObject(copyObjectInput)
			if err != nil {
				klog.Error("failed to copy object:", sourceKey, "->", destinationKey, ":", err)
				copyErrors = append(copyErrors, err)
			}
		}
		return true
	})

	if err != nil {
		return fmt.Errorf("failed to list objects: %v", err)
	}

	if len(copyErrors) > 0 {
		return fmt.Errorf("some objects failed to copy: %v", copyErrors)
	}

	return nil
}

func (c *S3Client) ListObjectWithPrefix(prefix *string, marker *string, delimiter *string, maxKeys *int64) (*s3.ListObjectsOutput, error) {
	s3Client, err := c.newClient()
	if err != nil {
		return nil, err
	}
	output, err := s3Client.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String(c.bucketName),
		Prefix:    prefix,
		Marker:    marker,
		MaxKeys:   maxKeys,
		Delimiter: delimiter,
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}
