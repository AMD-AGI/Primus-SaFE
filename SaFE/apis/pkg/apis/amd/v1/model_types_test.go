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
		// Single-segment canonical repositories.
		"https://huggingface.co/gpt2":               "gpt2",
		"https://huggingface.co/bert-base-uncased/": "bert-base-uncased",
		"t5-base": "t5-base",
		// Not model repositories.
		"https://huggingface.co/datasets/org/data": "my-model",
		"https://huggingface.co/datasets":          "my-model",
		"https://huggingface.co/spaces/org/app":    "my-model",
		"https://huggingface.co/":                  "my-model",
		"https://huggingface.co/a--b":              "my-model",
		"https://huggingface.co/-x":                "my-model",
		"https://example.com/gpt2":                 "my-model",
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

func TestModelHFRepoID(t *testing.T) {
	cases := map[string]string{
		"https://huggingface.co/gpt2":                "gpt2",
		"https://huggingface.co/EleutherAI/gpt-j-6b": "EleutherAI/gpt-j-6b",
		"huggingface.co/bert-base-uncased":           "bert-base-uncased",
		"Qwen/Qwen2.5-7B":                            "Qwen/Qwen2.5-7B",
		"https://huggingface.co/models":              "",
		"https://huggingface.co/datasets/org/data":   "",
		"https://huggingface.co/org/repo/tree/main":  "",
		"https://huggingface.co/org/re po":           "",
		"https://huggingface.co/org/..":              "",
		"s3://bucket/gpt2":                           "",
		"":                                           "",
	}
	for url, want := range cases {
		m := &Model{Spec: ModelSpec{Source: ModelSource{URL: url}}}
		if got := m.GetHFRepoID(); got != want {
			t.Errorf("GetHFRepoID(%q) = %q, want %q", url, got, want)
		}
	}
	// A single-segment name never contains "--", so it cannot take the directory of an
	// "owner/name" repository.
	single := &Model{Spec: ModelSpec{Source: ModelSource{URL: "https://huggingface.co/gpt2"}}}
	pair := &Model{Spec: ModelSpec{Source: ModelSource{URL: "https://huggingface.co/gpt/2"}}}
	if single.GetLocalDirName() == pair.GetLocalDirName() {
		t.Errorf("single-segment and owner/name repositories share directory %q", single.GetLocalDirName())
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
