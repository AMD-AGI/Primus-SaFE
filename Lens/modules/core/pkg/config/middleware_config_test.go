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
			name:            "未设置任何配置（默认启用）",
			config:          MiddlewareConfig{},
			expectedLogging: true,
			expectedTracing: true,
			description:     "当配置文件中没有middleware配置时，应该默认启用所有中间件",
		},
		{
			name: "显式启用logging",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(true),
			},
			expectedLogging: true,
			expectedTracing: true,
			description:     "显式设置logging为true，tracing未设置应默认为true",
		},
		{
			name: "显式禁用logging",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(false),
			},
			expectedLogging: false,
			expectedTracing: true,
			description:     "显式设置logging为false，tracing未设置应默认为true",
		},
		{
			name: "显式启用tracing",
			config: MiddlewareConfig{
				EnableTracing: boolPtr(true),
			},
			expectedLogging: true,
			expectedTracing: true,
			description:     "logging未设置应默认为true，显式设置tracing为true",
		},
		{
			name: "显式禁用tracing",
			config: MiddlewareConfig{
				EnableTracing: boolPtr(false),
			},
			expectedLogging: true,
			expectedTracing: false,
			description:     "logging未设置应默认为true，显式设置tracing为false",
		},
		{
			name: "全部启用",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(true),
				EnableTracing: boolPtr(true),
			},
			expectedLogging: true,
			expectedTracing: true,
			description:     "显式启用所有中间件",
		},
		{
			name: "全部禁用",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(false),
				EnableTracing: boolPtr(false),
			},
			expectedLogging: false,
			expectedTracing: false,
			description:     "显式禁用所有中间件",
		},
		{
			name: "启用logging，禁用tracing",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(true),
				EnableTracing: boolPtr(false),
			},
			expectedLogging: true,
			expectedTracing: false,
			description:     "混合配置场景1",
		},
		{
			name: "禁用logging，启用tracing",
			config: MiddlewareConfig{
				EnableLogging: boolPtr(false),
				EnableTracing: boolPtr(true),
			},
			expectedLogging: false,
			expectedTracing: true,
			description:     "混合配置场景2",
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

// boolPtr 返回bool指针的辅助函数
func boolPtr(b bool) *bool {
	return &b
}

