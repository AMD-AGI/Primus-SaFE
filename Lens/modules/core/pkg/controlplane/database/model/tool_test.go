// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"encoding/json"
	"testing"
)

func TestAppConfig_Value(t *testing.T) {
	tests := []struct {
		name    string
		config  AppConfig
		want    string
		wantErr bool
	}{
		{
			name:   "nil config",
			config: nil,
			want:   "{}",
		},
		{
			name: "skill config",
			config: AppConfig{
				"s3_key":    "skills/test/SKILL.md",
				"is_prefix": false,
			},
			want: `{"is_prefix":false,"s3_key":"skills/test/SKILL.md"}`,
		},
		{
			name: "mcp config",
			config: AppConfig{
				"mcpServers": map[string]interface{}{
					"test-server": map[string]interface{}{
						"command": "npx",
						"args":    []interface{}{"-y", "test-mcp"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.config.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("AppConfig.Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != "" && got != tt.want {
				t.Errorf("AppConfig.Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppConfig_Scan(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    AppConfig
		wantErr bool
	}{
		{
			name:  "nil value",
			value: nil,
			want:  AppConfig{},
		},
		{
			name:  "string value",
			value: `{"s3_key":"test.md"}`,
			want:  AppConfig{"s3_key": "test.md"},
		},
		{
			name:  "bytes value",
			value: []byte(`{"s3_key":"test.md"}`),
			want:  AppConfig{"s3_key": "test.md"},
		},
		{
			name:    "invalid type",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config AppConfig
			err := config.Scan(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("AppConfig.Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.want != nil {
				for k, v := range tt.want {
					if config[k] != v {
						t.Errorf("AppConfig.Scan() key %s = %v, want %v", k, config[k], v)
					}
				}
			}
		})
	}
}

func TestTool_GetSkillS3Key(t *testing.T) {
	tests := []struct {
		name    string
		tool    *Tool
		wantKey string
	}{
		{
			name: "skill with s3_key",
			tool: &Tool{
				Type: AppTypeSkill,
				Config: AppConfig{
					"s3_key":    "skills/test/SKILL.md",
					"is_prefix": false,
				},
			},
			wantKey: "skills/test/SKILL.md",
		},
		{
			name: "skill with prefix",
			tool: &Tool{
				Type: AppTypeSkill,
				Config: AppConfig{
					"s3_key":    "skills/test/",
					"is_prefix": true,
				},
			},
			wantKey: "skills/test/",
		},
		{
			name: "mcp type returns empty",
			tool: &Tool{
				Type: AppTypeMCP,
				Config: AppConfig{
					"command": "npx",
				},
			},
			wantKey: "",
		},
		{
			name: "skill without s3_key returns empty",
			tool: &Tool{
				Type:   AppTypeSkill,
				Config: AppConfig{},
			},
			wantKey: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey := tt.tool.GetSkillS3Key()
			if gotKey != tt.wantKey {
				t.Errorf("Tool.GetSkillS3Key() = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}

func TestTool_GetMCPServerConfig(t *testing.T) {
	tests := []struct {
		name        string
		tool        *Tool
		wantCommand string
		wantArgs    []string
		wantEnv     map[string]string
	}{
		{
			name: "mcp with mcpServers format",
			tool: &Tool{
				Type: AppTypeMCP,
				Config: AppConfig{
					"mcpServers": map[string]interface{}{
						"test-server": map[string]interface{}{
							"command": "npx",
							"args":    []interface{}{"-y", "test-mcp"},
							"env": map[string]interface{}{
								"NODE_ENV": "production",
							},
						},
					},
				},
			},
			wantCommand: "npx",
			wantArgs:    []string{"-y", "test-mcp"},
			wantEnv:     map[string]string{"NODE_ENV": "production"},
		},
		{
			name: "mcp with direct format (fallback)",
			tool: &Tool{
				Type: AppTypeMCP,
				Config: AppConfig{
					"command": "node",
					"args":    []interface{}{"server.js"},
				},
			},
			wantCommand: "node",
			wantArgs:    []string{"server.js"},
		},
		{
			name: "skill type returns empty",
			tool: &Tool{
				Type: AppTypeSkill,
				Config: AppConfig{
					"s3_key": "test.md",
				},
			},
			wantCommand: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmd, gotArgs, gotEnv := tt.tool.GetMCPServerConfig()
			if gotCmd != tt.wantCommand {
				t.Errorf("Tool.GetMCPServerConfig() command = %v, want %v", gotCmd, tt.wantCommand)
			}
			if len(gotArgs) != len(tt.wantArgs) {
				t.Errorf("Tool.GetMCPServerConfig() args = %v, want %v", gotArgs, tt.wantArgs)
			}
			if tt.wantEnv != nil {
				for k, v := range tt.wantEnv {
					if gotEnv[k] != v {
						t.Errorf("Tool.GetMCPServerConfig() env[%s] = %v, want %v", k, gotEnv[k], v)
					}
				}
			}
		})
	}
}

func TestTool_TableName(t *testing.T) {
	tool := &Tool{}
	if got := tool.TableName(); got != TableNameTools {
		t.Errorf("Tool.TableName() = %v, want %v", got, TableNameTools)
	}
}

func TestToolJSON(t *testing.T) {
	tool := &Tool{
		ID:          1,
		Type:        AppTypeSkill,
		Name:        "test-skill",
		DisplayName: "Test Skill",
		Description: "A test skill",
		Tags:        []string{"test", "demo"},
		IsPublic:    true,
		Status:      AppStatusActive,
		Config: AppConfig{
			"s3_key": "skills/test/SKILL.md",
		},
	}

	// Test JSON marshaling
	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("Failed to marshal Tool: %v", err)
	}

	// Test JSON unmarshaling
	var decoded Tool
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Tool: %v", err)
	}

	if decoded.Name != tool.Name {
		t.Errorf("Decoded Name = %v, want %v", decoded.Name, tool.Name)
	}
	if decoded.Type != tool.Type {
		t.Errorf("Decoded Type = %v, want %v", decoded.Type, tool.Type)
	}
}
