// Package workflow provides Temporal workflows for orchestrating multi-workspace
// Terraform deployments with dependency resolution and output propagation.
// It includes configuration types, validation logic, and the parent orchestrator workflow.
package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// InfrastructureConfig describes the overall set of workspaces to orchestrate.
// YAML and JSON tags support both CLI YAML configs and inline MCP JSON payloads.
type InfrastructureConfig struct {
	WorkspaceRoot string            `json:"workspace_root" yaml:"workspace_root"`
	Workspaces    []WorkspaceConfig `json:"workspaces" yaml:"workspaces"`
}

// WorkspaceConfig defines a single workspace/run target.
type WorkspaceConfig struct {
	Name       string         `json:"name" yaml:"name"`
	Kind       string         `json:"kind" yaml:"kind"`
	Dir        string         `json:"dir" yaml:"dir"`
	TFVars     string         `json:"tfvars,omitempty" yaml:"tfvars,omitempty"`
	DependsOn  []string       `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	Inputs     []InputMapping `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	TaskQueue  string         `json:"taskQueue,omitempty" yaml:"taskQueue,omitempty"`
	Operations []string       `json:"operations,omitempty" yaml:"operations,omitempty"`

	// ExtraVars are populated at runtime by the parent workflow
	// from resolved InputMappings. Values preserve their original JSON types
	// (string, number, bool, array, object) to match Terraform variable types.
	ExtraVars map[string]interface{} `json:"extraVars,omitempty" yaml:"extraVars,omitempty"`
}

// Signal names
const (
	SignalStartChild        = "start-child"
	SignalWorkspaceFinished = "workspace-finished"
	SignalShutdown          = "shutdown"
)

// StartChildSignal payload
type StartChildSignal struct {
	Workspace WorkspaceConfig
}

// WorkspaceFinishedSignal payload
type WorkspaceFinishedSignal struct {
	Name    string
	Outputs map[string]interface{}
}

// InputMapping defines how to map an output from a dependency workspace
// to a variable in the current workspace.
type InputMapping struct {
	SourceWorkspace string `json:"sourceWorkspace" yaml:"sourceWorkspace"`
	SourceOutput    string `json:"sourceOutput" yaml:"sourceOutput"`
	TargetVar       string `json:"targetVar" yaml:"targetVar"`
}

// NormalizeInfrastructureConfig applies defaults (e.g., kind) and resolves
// workspace-relative paths for directories and tfvars.
func NormalizeInfrastructureConfig(cfg InfrastructureConfig) InfrastructureConfig {
	root := cfg.WorkspaceRoot
	cwd, err := os.Getwd()
	if err != nil {
		// Fallback to current directory if we can't determine it
		cwd = "."
	}
	base := root
	if base == "" {
		base = "."
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(cwd, base)
	}

	for i, ws := range cfg.Workspaces {
		if ws.Kind == "" {
			ws.Kind = "terraform"
		}
		if !filepath.IsAbs(ws.Dir) {
			ws.Dir = filepath.Join(base, ws.Dir)
		}
		if ws.TFVars != "" && !filepath.IsAbs(ws.TFVars) {
			ws.TFVars = filepath.Join(base, ws.TFVars)
		}
		// Apply default operations if not specified
		if len(ws.Operations) == 0 {
			ws.Operations = getDefaultOperations(ws.Kind)
		}
		cfg.Workspaces[i] = ws
	}
	return cfg
}

// ValidateInfrastructureConfig performs structural checks (duplicates, cycles,
// missing dependencies, unsupported kinds). Keep it deterministic and pure so
// it can be reused by CLI/MCP before workflow execution.
func ValidateInfrastructureConfig(cfg InfrastructureConfig) error {
	if len(cfg.Workspaces) == 0 {
		return errors.New("no workspaces defined")
	}

	// index by name
	index := make(map[string]WorkspaceConfig, len(cfg.Workspaces))
	for _, ws := range cfg.Workspaces {
		if strings.TrimSpace(ws.Name) == "" {
			return errors.New("workspace name cannot be empty")
		}
		if _, exists := index[ws.Name]; exists {
			return fmt.Errorf("duplicate workspace name: %s", ws.Name)
		}
		if strings.TrimSpace(ws.Dir) == "" {
			return fmt.Errorf("workspace %s missing dir", ws.Name)
		}
		kind := ws.Kind
		if kind == "" {
			kind = "terraform"
		}
		if !isSupportedKind(kind) {
			return fmt.Errorf("unsupported kind %s for workspace %s", kind, ws.Name)
		}
		index[ws.Name] = ws
	}

	// cycle detection via DFS (deterministic using slice order)
	visiting := make(map[string]bool, len(index))
	visited := make(map[string]bool, len(index))

	var dfs func(name string) error
	dfs = func(name string) error {
		if visiting[name] {
			return fmt.Errorf("dependency cycle detected at workspace %s", name)
		}
		if visited[name] {
			return nil
		}
		visiting[name] = true
		ws := index[name]
		for _, dep := range ws.DependsOn {
			if err := dfs(dep); err != nil {
				return err
			}
		}
		visiting[name] = false
		visited[name] = true
		return nil
	}

	for _, ws := range cfg.Workspaces {
		if err := dfs(ws.Name); err != nil {
			return err
		}
	}

	// dependency existence and input mapping validation
	for _, ws := range cfg.Workspaces {
		for _, dep := range ws.DependsOn {
			if _, ok := index[dep]; !ok {
				return fmt.Errorf("workspace %s depends on unknown workspace %s", ws.Name, dep)
			}
			if dep == ws.Name {
				return fmt.Errorf("workspace %s cannot depend on itself", ws.Name)
			}
		}
		for _, input := range ws.Inputs {
			if _, ok := index[input.SourceWorkspace]; !ok {
				return fmt.Errorf("workspace %s input mapping source %s not found", ws.Name, input.SourceWorkspace)
			}
			// ensure the source workspace is actually a dependency (direct or transitive)
			if !isTransitivelyDependent(ws.Name, input.SourceWorkspace, index) {
				return fmt.Errorf("workspace %s must depend (directly or transitively) on %s to use its outputs in mapping", ws.Name, input.SourceWorkspace)
			}
		}
	}

	// Validate operations for each workspace
	for _, ws := range cfg.Workspaces {
		if err := ValidateWorkspaceOperations(ws); err != nil {
			return err
		}
	}

	return nil
}

// ValidateWorkspaceOperations validates that the operations list for a workspace
// is valid based on its kind (e.g., terraform requires init and validate).
func ValidateWorkspaceOperations(ws WorkspaceConfig) error {
	kind := ws.Kind
	if kind == "" {
		kind = "terraform"
	}

	// If no operations specified, use default based on kind
	if len(ws.Operations) == 0 {
		// Default is fine, will be handled by NormalizeInfrastructureConfig
		return nil
	}

	switch kind {
	case "terraform":
		return validateTerraformOperations(ws.Name, ws.Operations)
	default:
		return fmt.Errorf("workspace %s: validation not implemented for kind %s", ws.Name, kind)
	}
}

// validateTerraformOperations ensures terraform operations are valid and properly ordered.
func validateTerraformOperations(name string, operations []string) error {
	// Define valid operations for terraform
	validOps := map[string]bool{
		"init":     true,
		"validate": true,
		"plan":     true,
		"apply":    true,
	}

	// Check for unknown operations
	for _, op := range operations {
		if !validOps[op] {
			return fmt.Errorf("workspace %s: unknown operation '%s' for kind 'terraform'", name, op)
		}
	}

	// Check for required operations
	hasInit := false
	hasValidate := false
	hasPlan := false
	hasApply := false

	for _, op := range operations {
		switch op {
		case "init":
			hasInit = true
		case "validate":
			hasValidate = true
		case "plan":
			hasPlan = true
		case "apply":
			hasApply = true
		}
	}

	if !hasInit {
		return fmt.Errorf("workspace %s: operation 'init' is required for kind 'terraform'", name)
	}
	if !hasValidate {
		return fmt.Errorf("workspace %s: operation 'validate' is required for kind 'terraform'", name)
	}

	// Validate ordering constraints
	initIdx, validateIdx, planIdx, applyIdx := -1, -1, -1, -1
	for i, op := range operations {
		switch op {
		case "init":
			initIdx = i
		case "validate":
			validateIdx = i
		case "plan":
			planIdx = i
		case "apply":
			applyIdx = i
		}
	}

	// validate must come after init
	if validateIdx < initIdx {
		return fmt.Errorf("workspace %s: operation 'validate' must come after 'init'", name)
	}

	// plan must come after validate (if present)
	if hasPlan && planIdx < validateIdx {
		return fmt.Errorf("workspace %s: operation 'plan' must come after 'validate'", name)
	}

	// apply must come after plan (if present)
	if hasApply {
		if !hasPlan {
			return fmt.Errorf("workspace %s: operation 'apply' requires 'plan' to be present", name)
		}
		if applyIdx < planIdx {
			return fmt.Errorf("workspace %s: operation 'apply' must come after 'plan'", name)
		}
	}

	return nil
}

// isTransitivelyDependent returns true if target depends on source (directly or transitively)
func isTransitivelyDependent(target, source string, index map[string]WorkspaceConfig) bool {
	ws, ok := index[target]
	if !ok {
		return false
	}

	for _, dep := range ws.DependsOn {
		if dep == source {
			return true
		}
		if isTransitivelyDependent(dep, source, index) {
			return true
		}
	}
	return false
}

// CalculateDepths returns a map of workspace names to their depth in the DAG.
// Depth is defined as the length of the longest path from a root (no dependencies) to that node.
func CalculateDepths(workspaces []WorkspaceConfig) map[string]int {
	index := make(map[string]WorkspaceConfig)
	for _, ws := range workspaces {
		index[ws.Name] = ws
	}

	depths := make(map[string]int)
	var getDepth func(name string) int
	getDepth = func(name string) int {
		if d, ok := depths[name]; ok {
			return d
		}

		ws := index[name]
		if len(ws.DependsOn) == 0 {
			depths[name] = 0
			return 0
		}

		maxDepDepth := -1
		for _, dep := range ws.DependsOn {
			d := getDepth(dep)
			if d > maxDepDepth {
				maxDepDepth = d
			}
		}
		depths[name] = maxDepDepth + 1
		return depths[name]
	}

	for _, ws := range workspaces {
		getDepth(ws.Name)
	}

	return depths
}

func isSupportedKind(kind string) bool {
	switch kind {
	case "", "terraform":
		return true
	default:
		return false
	}
}

// getDefaultOperations returns the default operations list for a given kind.
func getDefaultOperations(kind string) []string {
	if kind == "" {
		kind = "terraform"
	}
	switch kind {
	case "terraform":
		return []string{"init", "validate", "plan", "apply"}
	default:
		return []string{}
	}
}

// LoadConfigFromFile reads and parses an infrastructure configuration file.
// Supports both YAML and JSON formats based on file extension.
func LoadConfigFromFile(path string) (InfrastructureConfig, error) {
	var config InfrastructureConfig

	body, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %v", err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(body, &config); err != nil {
			return config, fmt.Errorf("invalid YAML config: %v", err)
		}
	case ".json":
		if err := json.Unmarshal(body, &config); err != nil {
			return config, fmt.Errorf("invalid JSON config: %v", err)
		}
	default:
		// Try JSON as fallback for files without extension
		if err := json.Unmarshal(body, &config); err != nil {
			return config, fmt.Errorf("invalid config format (expected YAML or JSON): %v", err)
		}
	}

	return config, nil
}
