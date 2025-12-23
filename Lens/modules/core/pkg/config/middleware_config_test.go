package config

import "testing"

func TestMiddlewareConfig_DefaultValues(t *testing.T) {
	tests := []struct {
		name                string
		config              MiddlewareConfig
		expectedLogging     bool
		expectedTracing     bool
		description         string
	}{
		{
			name:            "No configuration set (default enabled)",
			config:          MiddlewareConfig{},
			expectedLogging: true,
			expectedTracing: true,
			description:     "When no middleware config in file, all middlewares should be enabled by default",
		},
		{
			name: "Explicitly enable logging",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(true),
			},
			expectedLogging: true,
			expectedTracing: true,
			description:     "Explicitly set logging to true, tracing unset should default to true",
		},
		{
			name: "Explicitly disable logging",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(false),
			},
			expectedLogging: false,
			expectedTracing: true,
			description:     "Explicitly set logging to false, tracing unset should default to true",
		},
		{
			name: "Explicitly enable tracing",
			config: MiddlewareConfig{
				EnableTracing: boolPtr(true),
			},
			expectedLogging: true,
			expectedTracing: true,
			description:     "Logging unset should default to true, explicitly set tracing to true",
		},
		{
			name: "Explicitly disable tracing",
			config: MiddlewareConfig{
				EnableTracing: boolPtr(false),
			},
			expectedLogging: true,
			expectedTracing: false,
			description:     "Logging unset should default to true, explicitly set tracing to false",
		},
		{
			name: "Enable all",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(true),
				EnableTracing: boolPtr(true),
			},
			expectedLogging: true,
			expectedTracing: true,
			description:     "Explicitly enable all middlewares",
		},
		{
			name: "Disable all",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(false),
				EnableTracing: boolPtr(false),
			},
			expectedLogging: false,
			expectedTracing: false,
			description:     "Explicitly disable all middlewares",
		},
		{
			name: "Enable logging, disable tracing",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(true),
				EnableTracing: boolPtr(false),
			},
			expectedLogging: true,
			expectedTracing: false,
			description:     "Mixed configuration scenario 1",
		},
		{
			name: "Disable logging, enable tracing",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(false),
				EnableTracing: boolPtr(true),
			},
			expectedLogging: false,
			expectedTracing: true,
			description:     "Mixed configuration scenario 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLogging := tt.config.IsLoggingEnabled()
			if gotLogging != tt.expectedLogging {
				t.Errorf("%s: IsLoggingEnabled() = %v, want %v", tt.description, gotLogging, tt.expectedLogging)
			}

			gotTracing := tt.config.IsTracingEnabled()
			if gotTracing != tt.expectedTracing {
				t.Errorf("%s: IsTracingEnabled() = %v, want %v", tt.description, gotTracing, tt.expectedTracing)
			}
		})
	}
}

// boolPtr is a helper function that returns a bool pointer
func boolPtr(b bool) *bool {
	return &b
}

