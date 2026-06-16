/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestParseS3PathStyleURL(t *testing.T) {
	_, err := parseS3PathStyleURL("")
	assert.Error(t, err)

	_, err = parseS3PathStyleURL("ftp://host/bucket/key")
	assert.Error(t, err)

	_, err = parseS3PathStyleURL("https://host")
	assert.Error(t, err) // missing path

	loc, err := parseS3PathStyleURL("https://s3.local/bucket/models/a.bin")
	assert.NoError(t, err)
	assert.Equal(t, "bucket", loc.Bucket)
	assert.Equal(t, "https://s3.local", loc.Endpoint)
	assert.Equal(t, "models/a.bin", loc.Key)

	loc, err = parseS3PathStyleURL("https://s3.local/onlybucket/")
	assert.NoError(t, err)
	assert.Equal(t, "onlybucket", loc.Bucket)
	assert.Equal(t, "", loc.Key)
}

func TestNewConfigFromCredentials(t *testing.T) {
	_, err := newConfigFromCredentials("", "sk", "http://e", "b")
	assert.Error(t, err)
	_, err = newConfigFromCredentials("ak", "", "http://e", "b")
	assert.Error(t, err)
	_, err = newConfigFromCredentials("ak", "sk", "", "b")
	assert.Error(t, err)
	_, err = newConfigFromCredentials("ak", "sk", "http://e", "")
	assert.Error(t, err)

	cfg, err := newConfigFromCredentials("ak", "sk", "http://e", "b")
	assert.NoError(t, err)
	assert.Equal(t, "b", *cfg.Bucket)
}

func TestNewConfigFromCredentialsURL(t *testing.T) {
	_, _, err := NewConfigFromCredentials("ak", "sk", "bad-url")
	assert.Error(t, err)

	cfg, loc, err := NewConfigFromCredentials("ak", "sk", "https://s3.local/bucket/key")
	assert.NoError(t, err)
	assert.Equal(t, "bucket", loc.Bucket)
	assert.Equal(t, "bucket", *cfg.Bucket)
}

func TestNewConfigGating(t *testing.T) {
	viper.Reset()
	// s3 disabled
	_, err := NewConfig()
	assert.Error(t, err)

	// enabled but empty bucket -> still error (bucket comes from secret file, unset)
	viper.Set("s3.enable", true)
	_, err = NewConfig()
	assert.Error(t, err)
}

func TestWithOptionalTimeout(t *testing.T) {
	ctx := context.Background()
	c1, cancel1 := WithOptionalTimeout(ctx, 0)
	defer cancel1()
	_, hasDeadline := c1.Deadline()
	assert.False(t, hasDeadline)

	c2, cancel2 := WithOptionalTimeout(ctx, 5)
	defer cancel2()
	dl, ok := c2.Deadline()
	assert.True(t, ok)
	assert.True(t, dl.After(time.Now()))
}
