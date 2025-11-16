package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultMetricsNamespace tests the default namespace constant
func TestDefaultMetricsNamespace(t *testing.T) {
	assert.Equal(t, "primus_lens", DefaultMetricsNamespace, "Default namespace should be 'primus_lens'")
	assert.NotEmpty(t, DefaultMetricsNamespace, "Default namespace should not be empty")
}

// TestDefaultMetricsNamespace_IsString tests that the constant is a string
func TestDefaultMetricsNamespace_IsString(t *testing.T) {
	var _ string = DefaultMetricsNamespace
	assert.IsType(t, "", DefaultMetricsNamespace)
}

// TestDefaultMetricsNamespace_NoSpaces tests that namespace has no spaces
func TestDefaultMetricsNamespace_NoSpaces(t *testing.T) {
	assert.NotContains(t, DefaultMetricsNamespace, " ", "Namespace should not contain spaces")
	assert.NotContains(t, DefaultMetricsNamespace, "\t", "Namespace should not contain tabs")
	assert.NotContains(t, DefaultMetricsNamespace, "\n", "Namespace should not contain newlines")
}

// TestDefaultMetricsNamespace_ValidFormat tests that namespace follows Prometheus naming conventions
func TestDefaultMetricsNamespace_ValidFormat(t *testing.T) {
	// Prometheus metric names should match [a-zA-Z_:][a-zA-Z0-9_:]*
	// For namespace, we typically use lowercase with underscores
	for _, char := range DefaultMetricsNamespace {
		isValid := (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_'
		assert.True(t, isValid, "Namespace should only contain lowercase letters, numbers, and underscores: got char '%c'", char)
	}
}

// TestDefaultMetricsNamespace_NotEmpty tests that the namespace is not empty
func TestDefaultMetricsNamespace_NotEmpty(t *testing.T) {
	assert.NotEqual(t, "", DefaultMetricsNamespace, "Default namespace should not be empty string")
	assert.Greater(t, len(DefaultMetricsNamespace), 0, "Default namespace length should be greater than 0")
}

// TestDefaultMetricsNamespace_Value tests the specific value
func TestDefaultMetricsNamespace_Value(t *testing.T) {
	// Verify the exact expected value
	expectedNamespace := "primus_lens"
	assert.Equal(t, expectedNamespace, DefaultMetricsNamespace, "Default namespace should be '%s'", expectedNamespace)
}

// TestDefaultMetricsNamespace_UsedInOpts tests that default namespace is used in opts
func TestDefaultMetricsNamespace_UsedInOpts(t *testing.T) {
	// Create opts without custom namespace
	opts := &mOpts{
		name: "test_metric",
		help: "test help",
	}

	// Verify default namespace is used
	counterOpts := opts.GetCounterOpts()
	assert.Equal(t, DefaultMetricsNamespace, counterOpts.Namespace)

	gaugeOpts := opts.GetGaugeOpts()
	assert.Equal(t, DefaultMetricsNamespace, gaugeOpts.Namespace)

	histogramOpts := opts.GetHistogramOpts()
	assert.Equal(t, DefaultMetricsNamespace, histogramOpts.Namespace)

	summaryOpts := opts.GetSummaryOpts()
	assert.Equal(t, DefaultMetricsNamespace, summaryOpts.Namespace)
}

// TestDefaultMetricsNamespace_Immutable tests that the constant cannot be modified
func TestDefaultMetricsNamespace_Immutable(t *testing.T) {
	// This is more of a compile-time check, but we can verify the value doesn't change
	originalValue := DefaultMetricsNamespace
	
	// Try to use it in various contexts
	_ = DefaultMetricsNamespace + "_suffix"
	
	// Verify it hasn't changed
	assert.Equal(t, originalValue, DefaultMetricsNamespace, "Default namespace should remain constant")
}

// TestDefaultMetricsNamespace_LengthReasonable tests that namespace length is reasonable
func TestDefaultMetricsNamespace_LengthReasonable(t *testing.T) {
	length := len(DefaultMetricsNamespace)
	assert.Greater(t, length, 3, "Namespace should be at least 4 characters")
	assert.Less(t, length, 100, "Namespace should be less than 100 characters")
}

// TestDefaultMetricsNamespace_StartsWithLetter tests that namespace starts with a letter
func TestDefaultMetricsNamespace_StartsWithLetter(t *testing.T) {
	if len(DefaultMetricsNamespace) > 0 {
		firstChar := DefaultMetricsNamespace[0]
		isLetter := (firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z')
		assert.True(t, isLetter, "Namespace should start with a letter, got: %c", firstChar)
	}
}

// TestDefaultMetricsNamespace_NoSpecialChars tests that namespace has no special characters
func TestDefaultMetricsNamespace_NoSpecialChars(t *testing.T) {
	specialChars := []string{"-", ".", "/", "\\", ":", ";", ",", "!", "@", "#", "$", "%", "^", "&", "*", "(", ")", "+", "=", "[", "]", "{", "}", "|", "<", ">", "?"}
	for _, char := range specialChars {
		assert.NotContains(t, DefaultMetricsNamespace, char, "Namespace should not contain special character: %s", char)
	}
}

