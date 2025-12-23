package workflow

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateInfrastructureConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     InfrastructureConfig
		wantErr bool
	}{
		{
			name:    "no workspaces",
			cfg:     InfrastructureConfig{},
			wantErr: true,
		},
		{
			name: "duplicate names",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a"},
					{Name: "a", Dir: "/tmp/b"},
				},
			},
			wantErr: true,
		},
		{
			name: "cycle detection",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a", DependsOn: []string{"b"}},
					{Name: "b", Dir: "/tmp/b", DependsOn: []string{"a"}},
				},
			},
			wantErr: true,
		},
		{
			name: "unsupported kind",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a", Kind: "helm"},
				},
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a"},
					{Name: "b", Dir: "/tmp/b", DependsOn: []string{"a"}},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid input mapping - source not found",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a"},
					{
						Name:      "b",
						Dir:       "/tmp/b",
						DependsOn: []string{"a"},
						Inputs: []InputMapping{
							{SourceWorkspace: "c", SourceOutput: "out", TargetVar: "var"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid input mapping - source not a dependency",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a"},
					{Name: "c", Dir: "/tmp/c"},
					{
						Name:      "b",
						Dir:       "/tmp/b",
						DependsOn: []string{"a"},
						Inputs: []InputMapping{
							{SourceWorkspace: "c", SourceOutput: "out", TargetVar: "var"},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid input mapping",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a"},
					{
						Name:      "b",
						Dir:       "/tmp/b",
						DependsOn: []string{"a"},
						Inputs: []InputMapping{
							{SourceWorkspace: "a", SourceOutput: "out", TargetVar: "var"},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid transitive input mapping",
			cfg: InfrastructureConfig{
				Workspaces: []WorkspaceConfig{
					{Name: "a", Dir: "/tmp/a"},
					{Name: "b", Dir: "/tmp/b", DependsOn: []string{"a"}},
					{
						Name:      "c",
						Dir:       "/tmp/c",
						DependsOn: []string{"b"},
						Inputs: []InputMapping{
							{SourceWorkspace: "a", SourceOutput: "out", TargetVar: "var"},
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		err := ValidateInfrastructureConfig(tt.cfg)
		if tt.wantErr {
			assert.Error(t, err, tt.name)
		} else {
			assert.NoError(t, err, tt.name)
		}
	}
}

func TestNormalizeInfrastructureConfig(t *testing.T) {
	cfg := InfrastructureConfig{
		WorkspaceRoot: "/root",
		Workspaces: []WorkspaceConfig{
			{Name: "a", Dir: "vpc", TFVars: "vpc.tfvars"},
		},
	}

	got := NormalizeInfrastructureConfig(cfg)
	assert.Equal(t, "/root/vpc", got.Workspaces[0].Dir)
	assert.Equal(t, "/root/vpc.tfvars", got.Workspaces[0].TFVars)
	assert.Equal(t, "terraform", got.Workspaces[0].Kind)
}

func TestCalculateDepths(t *testing.T) {
	workspaces := []WorkspaceConfig{
		{Name: "vpc", DependsOn: []string{}},
		{Name: "subnets", DependsOn: []string{"vpc"}},
		{Name: "eks", DependsOn: []string{"vpc", "subnets"}},
		{Name: "db", DependsOn: []string{"vpc"}},
		{Name: "app", DependsOn: []string{"eks", "db"}},
	}

	depths := CalculateDepths(workspaces)

	assert.Equal(t, 0, depths["vpc"])
	assert.Equal(t, 1, depths["subnets"])
	assert.Equal(t, 2, depths["eks"])
	assert.Equal(t, 1, depths["db"])
	assert.Equal(t, 3, depths["app"])
}

func TestLoadConfigFromFile_YAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yaml"

	yamlContent := `workspace_root: /test
workspaces:
  - name: vpc
    dir: terraform/vpc
    kind: terraform
  - name: subnets
    dir: terraform/subnets
    dependsOn:
      - vpc
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0o644)
	assert.NoError(t, err)

	cfg, err := LoadConfigFromFile(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "/test", cfg.WorkspaceRoot)
	assert.Equal(t, 2, len(cfg.Workspaces))
	assert.Equal(t, "vpc", cfg.Workspaces[0].Name)
	assert.Equal(t, "subnets", cfg.Workspaces[1].Name)
	assert.Equal(t, []string{"vpc"}, cfg.Workspaces[1].DependsOn)
}

func TestLoadConfigFromFile_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.json"

	jsonContent := `{
  "workspace_root": "/test",
  "workspaces": [
    {
      "name": "vpc",
      "dir": "terraform/vpc",
      "kind": "terraform"
    },
    {
      "name": "subnets",
      "dir": "terraform/subnets",
      "dependsOn": ["vpc"]
    }
  ]
}`
	err := os.WriteFile(configPath, []byte(jsonContent), 0o644)
	assert.NoError(t, err)

	cfg, err := LoadConfigFromFile(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "/test", cfg.WorkspaceRoot)
	assert.Equal(t, 2, len(cfg.Workspaces))
	assert.Equal(t, "vpc", cfg.Workspaces[0].Name)
	assert.Equal(t, "subnets", cfg.Workspaces[1].Name)
}

func TestLoadConfigFromFile_YMLExtension(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config.yml"

	yamlContent := `workspace_root: /test
workspaces:
  - name: vpc
    dir: terraform/vpc
`
	err := os.WriteFile(configPath, []byte(yamlContent), 0o644)
	assert.NoError(t, err)

	cfg, err := LoadConfigFromFile(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "/test", cfg.WorkspaceRoot)
	assert.Equal(t, 1, len(cfg.Workspaces))
}

func TestLoadConfigFromFile_FileNotFound(t *testing.T) {
	_, err := LoadConfigFromFile("/nonexistent/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read config file")
}

func TestLoadConfigFromFile_MalformedYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/bad.yaml"

	badYAML := `workspace_root: /test
workspaces:
  - name: vpc
    dir: terraform/vpc
  - invalid yaml here {{{{
`
	err := os.WriteFile(configPath, []byte(badYAML), 0o644)
	assert.NoError(t, err)

	_, err = LoadConfigFromFile(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid YAML config")
}

func TestLoadConfigFromFile_MalformedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/bad.json"

	badJSON := `{
  "workspace_root": "/test",
  "workspaces": [
    {
      "name": "vpc",
      "dir": "terraform/vpc"
    },
    invalid json here
  ]
}`
	err := os.WriteFile(configPath, []byte(badJSON), 0o644)
	assert.NoError(t, err)

	_, err = LoadConfigFromFile(configPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON config")
}

func TestLoadConfigFromFile_NoExtensionDefaultsToJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/config"

	jsonContent := `{
  "workspace_root": "/test",
  "workspaces": [
    {
      "name": "vpc",
      "dir": "terraform/vpc"
    }
  ]
}`
	err := os.WriteFile(configPath, []byte(jsonContent), 0o644)
	assert.NoError(t, err)

	cfg, err := LoadConfigFromFile(configPath)
	assert.NoError(t, err)
	assert.Equal(t, "/test", cfg.WorkspaceRoot)
	assert.Equal(t, 1, len(cfg.Workspaces))
}

func TestValidateWorkspaceOperations(t *testing.T) {
	tests := []struct {
		name    string
		ws      WorkspaceConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid operations - plan only",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "validate", "plan"},
			},
			wantErr: false,
		},
		{
			name: "valid operations - full apply",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "validate", "plan", "apply"},
			},
			wantErr: false,
		},
		{
			name: "missing init",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"validate", "plan"},
			},
			wantErr: true,
			errMsg:  "operation 'init' is required",
		},
		{
			name: "missing validate",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "plan"},
			},
			wantErr: true,
			errMsg:  "operation 'validate' is required",
		},
		{
			name: "unknown operation",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "validate", "plan", "destroy"},
			},
			wantErr: true,
			errMsg:  "unknown operation 'destroy'",
		},
		{
			name: "wrong order - validate before init",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"validate", "init", "plan"},
			},
			wantErr: true,
			errMsg:  "must come after 'init'",
		},
		{
			name: "wrong order - plan before validate",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "plan", "validate"},
			},
			wantErr: true,
			errMsg:  "must come after 'validate'",
		},
		{
			name: "apply without plan",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "validate", "apply"},
			},
			wantErr: true,
			errMsg:  "requires 'plan' to be present",
		},
		{
			name: "wrong order - apply before plan",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{"init", "validate", "apply", "plan"},
			},
			wantErr: true,
			errMsg:  "must come after 'plan'",
		},
		{
			name: "empty operations - should pass (defaults will be applied)",
			ws: WorkspaceConfig{
				Name:       "test",
				Kind:       "terraform",
				Dir:        "/tmp/test",
				Operations: []string{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkspaceOperations(tt.ws)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNormalizeInfrastructureConfig_DefaultOperations(t *testing.T) {
	cfg := InfrastructureConfig{
		WorkspaceRoot: "/root",
		Workspaces: []WorkspaceConfig{
			{Name: "a", Dir: "vpc"},
			{Name: "b", Dir: "subnets", Operations: []string{"init", "validate", "plan"}},
		},
	}

	got := NormalizeInfrastructureConfig(cfg)
	
	// First workspace should get default operations
	assert.Equal(t, []string{"init", "validate", "plan", "apply"}, got.Workspaces[0].Operations)
	
	// Second workspace should keep its explicit operations
	assert.Equal(t, []string{"init", "validate", "plan"}, got.Workspaces[1].Operations)
}

func TestValidateInfrastructureConfig_WithOperations(t *testing.T) {
	// Valid config with operations
	validCfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{
				Name:       "a",
				Dir:        "/tmp/a",
				Operations: []string{"init", "validate", "plan", "apply"},
			},
		},
	}
	assert.NoError(t, ValidateInfrastructureConfig(validCfg))

	// Invalid config - missing required operation
	invalidCfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{
				Name:       "a",
				Dir:        "/tmp/a",
				Operations: []string{"init", "plan"}, // missing validate
			},
		},
	}
	err := ValidateInfrastructureConfig(invalidCfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "operation 'validate' is required")
}

func TestIsTransitivelyDependent_NonExistentTarget(t *testing.T) {
	index := map[string]WorkspaceConfig{
		"a": {Name: "a", Dir: "/tmp/a"},
		"b": {Name: "b", Dir: "/tmp/b", DependsOn: []string{"a"}},
	}

	// Non-existent target should return false
	result := isTransitivelyDependent("nonexistent", "a", index)
	assert.False(t, result)
}

func TestGetDefaultOperations_UnknownKind(t *testing.T) {
	// Unknown kind should return empty slice
	ops := getDefaultOperations("helm")
	assert.Empty(t, ops)
}

func TestNormalizeInfrastructureConfig_AbsolutePaths(t *testing.T) {
	cfg := InfrastructureConfig{
		WorkspaceRoot: "/root",
		Workspaces: []WorkspaceConfig{
			{
				Name:   "a",
				Dir:    "/absolute/path/vpc",
				TFVars: "/absolute/path/vpc.tfvars",
			},
		},
	}

	got := NormalizeInfrastructureConfig(cfg)
	// Absolute paths should be preserved
	assert.Equal(t, "/absolute/path/vpc", got.Workspaces[0].Dir)
	assert.Equal(t, "/absolute/path/vpc.tfvars", got.Workspaces[0].TFVars)
}

func TestValidateInfrastructureConfig_EmptyWorkspaceName(t *testing.T) {
	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "", Dir: "/tmp/test"},
		},
	}

	err := ValidateInfrastructureConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "name cannot be empty")
}

func TestValidateInfrastructureConfig_MissingDir(t *testing.T) {
	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "test", Dir: ""},
		},
	}

	err := ValidateInfrastructureConfig(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing dir")
}

func TestValidateInfrastructureConfig_SelfDependency(t *testing.T) {
	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "a", Dir: "/tmp/a", DependsOn: []string{"a"}},
		},
	}

	err := ValidateInfrastructureConfig(cfg)
	assert.Error(t, err)
	// Self-dependency is detected as a cycle
	assert.Contains(t, err.Error(), "cycle")
}
