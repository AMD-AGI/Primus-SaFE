/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

import "testing"

func TestModelLocalDirName(t *testing.T) {
	cases := map[string]string{
		"https://huggingface.co/Qwen/Qwen2.5-7B-Instruct": "Qwen--Qwen2.5-7B-Instruct",
		"https://huggingface.co/a-b/c/":                   "a-b--c",
		"Qwen/Qwen2.5-7B":                                 "Qwen--Qwen2.5-7B",
		"s3://bucket/prefix":                              "my-model",
		"https://huggingface.co/a/b/c":                    "my-model",
		"https://huggingface.co/../etc":                   "my-model",
	}
	for url, want := range cases {
		m := &Model{Spec: ModelSpec{DisplayName: "my-model", Source: ModelSource{URL: url}}}
		if got := m.GetLocalDirName(); got != want {
			t.Errorf("GetLocalDirName(%q) = %q, want %q", url, got, want)
		}
	}
	// "owner/name" pairs that differ must not share a directory.
	a := &Model{Spec: ModelSpec{Source: ModelSource{URL: "https://huggingface.co/a-b/c"}}}
	b := &Model{Spec: ModelSpec{Source: ModelSource{URL: "https://huggingface.co/a/b-c"}}}
	if a.GetLocalDirName() == b.GetLocalDirName() {
		t.Errorf("distinct repositories share directory %q", a.GetLocalDirName())
	}
}

func TestBuildModelLocalPath(t *testing.T) {
	cases := []struct{ root, subpath, want string }{
		{"/data/", "", "/data/models/x"},
		{"/data", "/team/", "/data/team/models/x"},
		{"/data", "team/models/hf", "/data/team/models/hf/x"},
		{"/data", "models", "/data/models/x"},
		{"/data", "a/./b", "/data/a/b/models/x"},
		{"/data", "a/../..", "/data/models/x"},
	}
	for _, c := range cases {
		if got := BuildModelLocalPath(c.root, c.subpath, "x"); got != c.want {
			t.Errorf("BuildModelLocalPath(%q, %q) = %q, want %q", c.root, c.subpath, got, c.want)
		}
	}
}
