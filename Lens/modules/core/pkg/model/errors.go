package model

import "fmt"

// ConfigError represents a configuration error
type ConfigError struct {
	Message string
}

func (e ConfigError) Error() string {
	return e.Message
}

// ErrInvalidConfig creates an invalid configuration error
func ErrInvalidConfig(message string) error {
	return ConfigError{Message: fmt.Sprintf("invalid config: %s", message)}
}

