package detection

import "errors"

var (
	// Configuration errors
	ErrInvalidFrameworkName = errors.New("invalid framework name")
	ErrInvalidFrameworkType = errors.New("invalid framework type: must be 'training' or 'inference'")
	ErrInvalidPriority      = errors.New("invalid priority")
	ErrInvalidPatternName   = errors.New("invalid pattern name")
	ErrInvalidPattern       = errors.New("invalid pattern")
	ErrInvalidConfidence    = errors.New("invalid confidence")
	ErrConfigNotFound       = errors.New("config not found")
	ErrConfigParseFailed    = errors.New("config parse failed")

	// Pattern matching errors
	ErrPatternCompileFailed = errors.New("pattern compile failed")
	ErrNoPatternMatched     = errors.New("no pattern matched")

	// Framework detection errors
	ErrFrameworkNotDetected = errors.New("framework not detected")
	ErrNoMatcherFound       = errors.New("no matcher found")
)
