/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_prewarm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreloadScriptUsesDecimalBytesAndParallelReaders(t *testing.T) {
	script := preloadScript("/models/glm", "*.safetensors", 4)
	assert.Contains(t, script, `printf "%.0f\n"`)
	assert.Contains(t, script, `xargs -0 -n 1 -P"$PAR"`)
	assert.Contains(t, script, `FILE_COUNT=$(find "$MODEL_PATH" -type f -name "$GLOB" | wc -l)`)
	assert.Contains(t, script, "set -o pipefail")
	assert.True(t, strings.Contains(script, "symlinks excluded"))
}

func TestBuildConditionalResultPatch(t *testing.T) {
	jobUID := "abc-123"
	requestValue := `{"opsJobId":"job-1"}`
	resultValue := `{"opsJobId":"job-1","phase":"Running"}`

	addPatch, err := buildConditionalResultPatch(jobUID, requestValue, resultValue, false)
	assert.NoError(t, err)
	assert.Contains(t, string(addPatch), `"op":"test"`)
	assert.Contains(t, string(addPatch), `"op":"add"`)

	replacePatch, err := buildConditionalResultPatch(jobUID, requestValue, resultValue, true)
	assert.NoError(t, err)
	assert.Contains(t, string(replacePatch), `"op":"replace"`)
}

func TestAnnotationJSONPointerEscapesSlashes(t *testing.T) {
	assert.Equal(t, "/metadata/annotations/a~1b", annotationJSONPointer("a/b"))
}
