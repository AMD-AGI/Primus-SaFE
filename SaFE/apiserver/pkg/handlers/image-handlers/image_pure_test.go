/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func TestParseImageTag(t *testing.T) {
	host, repo, tag, err := parseImageTag("harbor.example.com/project/app:v1.0")
	assert.NoError(t, err)
	assert.Equal(t, "harbor.example.com", host)
	assert.Equal(t, "project/app", repo)
	assert.Equal(t, "v1.0", tag)

	_, _, _, err = parseImageTag("noslash")
	assert.Error(t, err)

	_, _, _, err = parseImageTag("host/repo-without-tag")
	assert.Error(t, err)
}

func TestParseImageID(t *testing.T) {
	id, err := parseImageID("123")
	assert.NoError(t, err)
	assert.Equal(t, int32(123), id)

	_, err = parseImageID("")
	assert.Error(t, err)

	_, err = parseImageID("abc")
	assert.Error(t, err)
}

func TestDeserializeParams(t *testing.T) {
	assert.Nil(t, deserializeParams(""))
	assert.Nil(t, deserializeParams("{"))

	got := deserializeParams("{workload:wl-1,image:img-1}")
	assert.Len(t, got, 2)
	assert.Equal(t, "workload", got[0].Name)
	assert.Equal(t, "wl-1", got[0].Value)
	assert.Equal(t, "image", got[1].Name)
	assert.Equal(t, "img-1", got[1].Value)

	// Quoted entries are trimmed.
	got = deserializeParams(`{"label:val"}`)
	assert.Len(t, got, 1)
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
	assert.NoError(t, err)
	assert.Equal(t, "Custom", project)
	assert.Equal(t, "rocm/7.0-preview", repo)
	assert.Equal(t, "20250112", ref)

	_, _, _, err = parseHarborImageName("noslash")
	assert.Error(t, err)

	_, _, _, err = parseHarborImageName("host/path-without-tag")
	assert.Error(t, err)

	_, _, _, err = parseHarborImageName("host/onlyproject:tag")
	assert.Error(t, err)
}

func TestTransEnvMapToEnv(t *testing.T) {
	envs := transEnvMapToEnv(map[string]string{"A": "1", "B": "2"})
	assert.Len(t, envs, 2)
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
	assert.NoError(t, decodeJsonb(src, &rd))
	assert.Equal(t, "sha256:abc", rd.Digest)
	assert.Equal(t, int64(100), rd.Size)
}

func TestNewHTTPClientSkipTLS(t *testing.T) {
	c := newHTTPClientSkipTLS()
	assert.NotNil(t, c)
	assert.Equal(t, 8*time.Second, c.Timeout)
}

func TestBuildImportLogSearchBody(t *testing.T) {
	body := buildImportLogSearchBody("job-1", time.Now().Add(-time.Hour), time.Now(), 100, "asc")
	assert.NotEmpty(t, body)
	assert.Contains(t, string(body), "job-1")
}

func TestBuildExportImageJobQuery(t *testing.T) {
	q := &ImageServiceRequest{}
	sqlizer, orderBy := buildExportImageJobQuery(q)
	assert.NotNil(t, sqlizer)
	assert.Len(t, orderBy, 1)

	q2 := &ImageServiceRequest{UserName: "u1", Ready: true, Workload: "wl-1", Order: "asc"}
	sqlizer2, orderBy2 := buildExportImageJobQuery(q2)
	assert.NotNil(t, sqlizer2)
	assert.Contains(t, orderBy2[0], "ASC")
}

func TestBuildPrewarmImageJobQuery(t *testing.T) {
	q := &ImageServiceRequest{}
	sqlizer, orderBy := buildPrewarmImageJobQuery(q)
	assert.NotNil(t, sqlizer)
	assert.Len(t, orderBy, 1)

	running := &ImageServiceRequest{Image: "img", Workspace: "ws", Status: "Running", UserName: "u", Ready: true}
	s2, _ := buildPrewarmImageJobQuery(running)
	assert.NotNil(t, s2)

	other := &ImageServiceRequest{Status: "Succeeded"}
	s3, _ := buildPrewarmImageJobQuery(other)
	assert.NotNil(t, s3)
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
	assert.GreaterOrEqual(t, len(res), 2)
	var appItem *GetImageResponseItem
	for i := range res {
		if res[i].Repo == "proj/app" {
			appItem = &res[i]
		}
	}
	assert.NotNil(t, appItem)
	assert.Len(t, appItem.Artifacts, 2)
}
