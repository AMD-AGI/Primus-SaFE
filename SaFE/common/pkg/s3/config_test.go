/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package s3

import (
	"testing"

	"gotest.tools/assert"
)

func TestParseS3PathStyleURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		wantBucket   string
		wantKey      string
		wantEndpoint string
		wantErr      bool
	}{
		{
			name:         "valid URL with file key",
			url:          "https://s3.example.com/mybucket/path/to/file.tar",
			wantBucket:   "mybucket",
			wantKey:      "path/to/file.tar",
			wantEndpoint: "https://s3.example.com",
			wantErr:      false,
		},
		{
			name:         "valid URL with directory prefix",
			url:          "https://s3.example.com/mybucket/models/llama/",
			wantBucket:   "mybucket",
			wantKey:      "models/llama/",
			wantEndpoint: "https://s3.example.com",
			wantErr:      false,
		},
		{
			name:         "valid URL with bucket only (trailing slash)",
			url:          "https://s3.example.com/mybucket/",
			wantBucket:   "mybucket",
			wantKey:      "",
			wantEndpoint: "https://s3.example.com",
			wantErr:      false,
		},
		{
			name:         "valid URL with bucket only (no trailing slash)",
			url:          "https://s3.example.com/mybucket",
			wantBucket:   "mybucket",
			wantKey:      "",
			wantEndpoint: "https://s3.example.com",
			wantErr:      false,
		},
		{
			name:         "valid HTTP URL",
			url:          "http://localhost:9000/testbucket/data.json",
			wantBucket:   "testbucket",
			wantKey:      "data.json",
			wantEndpoint: "http://localhost:9000",
			wantErr:      false,
		},
		{
			name:    "empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "invalid scheme (s3://)",
			url:     "s3://mybucket/key",
			wantErr: true,
		},
		{
			name:    "invalid scheme (ftp://)",
			url:     "ftp://s3.example.com/mybucket/key",
			wantErr: true,
		},
		{
			name:    "missing host",
			url:     "https:///mybucket/key",
			wantErr: true,
		},
		{
			name:    "missing bucket (root path only)",
			url:     "https://s3.example.com/",
			wantErr: true,
		},
		{
			name:    "missing path entirely",
			url:     "https://s3.example.com",
			wantErr: true,
		},
		{
			name:         "URL with special characters in key",
			url:          "https://s3.example.com/mybucket/path/file%20name.tar",
			wantBucket:   "mybucket",
			wantKey:      "path/file%20name.tar",
			wantEndpoint: "https://s3.example.com",
			wantErr:      false,
		},
		{
			name:         "URL with nested directories",
			url:          "https://s3.example.com/mybucket/a/b/c/d/e/file.txt",
			wantBucket:   "mybucket",
			wantKey:      "a/b/c/d/e/file.txt",
			wantEndpoint: "https://s3.example.com",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := parseS3PathStyleURL(tt.url)

			if tt.wantErr {
				assert.Assert(t, err != nil, "expected error but got none")
				return
			}

			assert.NilError(t, err)
			assert.Equal(t, tt.wantBucket, loc.Bucket)
			assert.Equal(t, tt.wantKey, loc.Key)
			assert.Equal(t, tt.wantEndpoint, loc.Endpoint)
		})
	}
}
