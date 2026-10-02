/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package imagedigest

import (
	"strings"
	"testing"
)

func TestIsPinned(t *testing.T) {
	digest := "repo/img@sha256:" + strings.Repeat("a", 64)
	if !IsPinned(digest) {
		t.Fatalf("expected pinned: %s", digest)
	}
	if IsPinned("repo/img:tag") {
		t.Fatal("tag must not be pinned")
	}
	if IsPinned("repo/img@sha256:short") {
		t.Fatal("short digest must not be pinned")
	}
}

func TestKeychainFromDockerConfigJSON(t *testing.T) {
	raw := []byte(`{"auths":{"example.com":{"username":"u","password":"p"}}}`)
	kc, err := KeychainFromDockerConfigJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if kc == nil {
		t.Fatal("nil keychain")
	}
}
