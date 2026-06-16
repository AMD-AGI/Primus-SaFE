/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/assert"
)

func newTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	cfg, err := newConfigFromCredentials("ak", "sk", endpoint, "bucket")
	assert.NoError(t, err)
	s3c := awss3.NewFromConfig(cfg.Config, func(o *awss3.Options) { o.UsePathStyle = true })
	return &Client{Config: cfg, opt: Option{}, s3Client: s3c}
}

func TestNilClientGuards(t *testing.T) {
	var c *Client
	ctx := context.Background()
	_, err := c.CreateMultiPartUpload(ctx, "k", 0)
	assert.Error(t, err)
	assert.Error(t, c.MultiPartUpload(ctx, &MultiUploadParam{}, 0))
	_, err = c.CompleteMultiPartUpload(ctx, &MultiUploadParam{}, 0)
	assert.Error(t, err)
	assert.Error(t, c.AbortMultiPartUpload(ctx, &MultiUploadParam{}, 0))
	_, err = c.PutObject(ctx, "k", "v", 0)
	assert.Error(t, err)
	assert.Error(t, c.PutObjectMultipart(ctx, "k", strings.NewReader("x"), 1))
	assert.Error(t, c.DeleteObject(ctx, "k", 0))
	_, err = c.GetObject(ctx, "k", 0)
	assert.Error(t, err)
	_, err = c.PresignModelFiles(ctx, "p", 1)
	assert.Error(t, err)
	_, err = c.ListObjectsWithSize(ctx, "p")
	assert.Error(t, err)
	assert.Error(t, c.DownloadFile(ctx, "k", "/tmp"))
	assert.Error(t, c.DownloadDirectory(ctx, "p", "/tmp"))
}

func TestInputValidation(t *testing.T) {
	c := newTestClient(t, "http://127.0.0.1:0")
	ctx := context.Background()
	_, err := c.PutObject(ctx, "", "v", 0)
	assert.Error(t, err)
	_, err = c.PutObject(ctx, "k", "", 0)
	assert.Error(t, err)
	assert.Error(t, c.DeleteObject(ctx, "", 0))
	_, err = c.GetObject(ctx, "", 0)
	assert.Error(t, err)
	assert.Error(t, c.PutObjectMultipart(ctx, "", strings.NewReader("x"), 1))

	// empty completed parts -> early return without network
	_, err = c.CompleteMultiPartUpload(ctx, &MultiUploadParam{}, 0)
	assert.NoError(t, err)
	assert.NoError(t, c.AbortMultiPartUpload(ctx, &MultiUploadParam{}, 0))
}

func TestPresignedURLs(t *testing.T) {
	c := newTestClient(t, "http://s3.local")
	ctx := context.Background()
	u, err := c.GeneratePresignedURL(ctx, "models/a.bin", 1)
	assert.NoError(t, err)
	assert.Contains(t, u, "a.bin")
	u, err = c.GeneratePresignedPutURL(ctx, "models/a.bin", 1)
	assert.NoError(t, err)
	assert.Contains(t, u, "a.bin")
}

func TestClientOperationsViaHTTPTest(t *testing.T) {
	listXML := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
<Name>bucket</Name><Prefix>p/</Prefix><KeyCount>2</KeyCount><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
<Contents><Key>p/a.bin</Key><Size>10</Size></Contents>
<Contents><Key>p/</Key><Size>0</Size></Contents>
</ListBucketResult>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "list-type"):
			w.Header().Set("Content-Type", "application/xml")
			w.Write([]byte(listXML))
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Length", "5")
			w.Write([]byte("hello"))
		case r.Method == http.MethodPut:
			w.Header().Set("ETag", `"etag123"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	_, err := c.PutObject(ctx, "k", "v", 30)
	assert.NoError(t, err)

	got, err := c.GetObject(ctx, "k", 30)
	assert.NoError(t, err)
	assert.Equal(t, "hello", got)

	assert.NoError(t, c.DeleteObject(ctx, "k", 30))

	files, err := c.ListObjectsWithSize(ctx, "p/")
	assert.NoError(t, err)
	assert.Len(t, files, 1) // directory marker skipped
	assert.Equal(t, "a.bin", files[0].Key)

	urls, err := c.PresignModelFiles(ctx, "p/", 1)
	assert.NoError(t, err)
	assert.Contains(t, urls, "a.bin")

	// MultiPartUpload uses PUT (UploadPart) then CompleteMultiPartUpload
	param := &MultiUploadParam{Key: "k", UploadId: "u1", PartNumber: 1, Value: "data"}
	assert.NoError(t, c.MultiPartUpload(ctx, param, 30))
	assert.Len(t, param.CompletedParts, 1)
}
