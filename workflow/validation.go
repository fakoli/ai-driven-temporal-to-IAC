package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fakoli/temporal-terraform-orchestrator/activities"
	"github.com/fakoli/temporal-terraform-orchestrator/validation"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ValidationConfig controls tfvars validation behavior
type ValidationConfig struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`               // Enable/disable validation
	RulesPath     string `json:"rulesPath" yaml:"rulesPath"`           // Custom rules path
	FailOnWarning bool   `json:"failOnWarning" yaml:"failOnWarning"`   // Treat warnings as errors
	SkipOnMissing bool   `json:"skipOnMissing" yaml:"skipOnMissing"`   // Skip if no tfvars
}

// DefaultValidationConfig returns the default validation configuration
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		Enabled:       true,
		RulesPath:     validation.DefaultRulesPath,
		FailOnWarning: false,
		SkipOnMissing: true,
	}
}

// ValidateTFVarsInWorkflow validates tfvars before executing terraform operations
// This should be called at the beginning of TerraformWorkflow
func ValidateTFVarsInWorkflow(ctx workflow.Context, ws WorkspaceConfig, runID string, validationCfg ValidationConfig) error {
	// Skip if validation is disabled
	if !validationCfg.Enabled {
		workflow.GetLogger(ctx).Info("TFVars validation disabled", "workspace", ws.Name)
		return nil
	}

	// Check if we have tfvars to validate
	hasTFVars := ws.TFVars != "" || len(ws.ExtraVars) > 0
	if !hasTFVars {
		if validationCfg.SkipOnMissing {
			workflow.GetLogger(ctx).Info("No tfvars to validate, skipping", "workspace", ws.Name)
			return nil
		}
		// If we require tfvars but don't have them, that's an error
		return fmt.Errorf("workspace %s has no tfvars to validate and skipOnMissing is false", ws.Name)
	}

	// Set up activity options for validation
	options := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2, // Validation is deterministic, few retries needed
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	// Create combined tfvars for validation
	// We need to merge the base tfvars with extra vars just like terraform does
	tfvars, err := loadAndMergeTFVars(ws, runID)
	if err != nil {
		return fmt.Errorf("failed to prepare tfvars for validation: %w", err)
	}

	// Prepare validation parameters
	params := activities.ValidateTFVarsParams{
		TFVars:        tfvars,
		WorkspaceName: ws.Name,
		WorkspaceKind: ws.Kind,
		WorkspaceDir:  ws.Dir,
		RulesPath:     validationCfg.RulesPath,
	}

	// Execute validation activity
	var validationActivities *activities.ValidationActivities
	var result activities.ValidateTFVarsResult

	err = workflow.ExecuteActivity(ctx, validationActivities.ValidateTFVars, params).Get(ctx, &result)
	if err != nil {
		return fmt.Errorf("validation activity failed: %w", err)
	}

	// Check validation result
	if !result.Valid {
		errMsg := activities.FormatValidationError(result)
		workflow.GetLogger(ctx).Error("TFVars validation failed",
			"workspace", ws.Name,
			"errors", result.ErrorCount,
			"warnings", result.WarningCount,
		)
		return fmt.Errorf("tfvars validation failed for workspace %s:\n%s", ws.Name, errMsg)
	}

	// Check for warnings if failOnWarning is set
	if validationCfg.FailOnWarning && result.WarningCount > 0 {
		errMsg := activities.FormatValidationError(result)
		return fmt.Errorf("tfvars validation has warnings (failOnWarning=true) for workspace %s:\n%s", ws.Name, errMsg)
	}

	workflow.GetLogger(ctx).Info("TFVars validation passed",
		"workspace", ws.Name,
		"warnings", result.WarningCount,
	)

	return nil
}

// loadAndMergeTFVars loads tfvars from file and merges with extra vars
// This mirrors the logic in activities.createCombinedTFVars but for workflow context
func loadAndMergeTFVars(ws WorkspaceConfig, runID string) (map[string]interface{}, error) {
	tfvars := make(map[string]interface{})

	// Load base tfvars if specified
	if ws.TFVars != "" {
		data, err := os.ReadFile(ws.TFVars)
		if err != nil {
			return nil, fmt.Errorf("failed to read tfvars file %s: %w", ws.TFVars, err)
		}

		// Determine format by extension
		ext := filepath.Ext(ws.TFVars)
		if ext == ".json" {
			if err := json.Unmarshal(data, &tfvars); err != nil {
				return nil, fmt.Errorf("failed to parse JSON tfvars: %w", err)
			}
		} else {
			// For HCL, we'll need to use the same parsing logic
			// For now, we'll support JSON primarily
			// TODO: Add HCL parsing using hashicorp/hcl/v2
			return nil, fmt.Errorf("HCL tfvars parsing not yet supported in validation, use .json format")
		}
	}

	// Merge extra vars (they take precedence)
	for key, value := range ws.ExtraVars {
		tfvars[key] = value
	}

	return tfvars, nil
}

// ValidateWorkspaceBeforeExecution is a helper to validate a single workspace
// Can be used standalone or as part of orchestration
func ValidateWorkspaceBeforeExecution(ws WorkspaceConfig, rulesPath string) (*validation.ValidationResult, error) {
	// Create validation service
	svc, err := validation.NewService(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create validation service: %w", err)
	}

	// Load tfvars
	tfvars := make(map[string]interface{})
	if ws.TFVars != "" {
		data, err := os.ReadFile(ws.TFVars)
		if err != nil {
			return nil, fmt.Errorf("failed to read tfvars: %w", err)
		}
		if err := json.Unmarshal(data, &tfvars); err != nil {
			return nil, fmt.Errorf("failed to parse tfvars: %w", err)
		}
	}

	// Merge extra vars
	for k, v := range ws.ExtraVars {
		tfvars[k] = v
	}

	// Create workspace context
	wsCtx := validation.WorkspaceContext{
		Name: ws.Name,
		Kind: ws.Kind,
		Dir:  ws.Dir,
	}
	if wsCtx.Kind == "" {
		wsCtx.Kind = "terraform"
	}

	// Validate
	result := svc.ValidateTFVars(tfvars, wsCtx)
	return &result, nil
}

// ValidateAllWorkspaces validates all workspaces in a configuration
func ValidateAllWorkspaces(cfg InfrastructureConfig, rulesPath string) (map[string]*validation.ValidationResult, error) {
	results := make(map[string]*validation.ValidationResult)

	for _, ws := range cfg.Workspaces {
		result, err := ValidateWorkspaceBeforeExecution(ws, rulesPath)
		if err != nil {
			// Create an error result
			errResult := validation.NewValidationResult()
			errResult.AddError(validation.ValidationIssue{
				Message:  err.Error(),
				Severity: validation.SeverityError,
			})
			results[ws.Name] = &errResult
		} else {
			results[ws.Name] = result
		}
	}

	return results, nil
}

// HasValidationErrors checks if any workspace has validation errors
func HasValidationErrors(results map[string]*validation.ValidationResult) bool {
	for _, result := range results {
		if !result.Valid {
			return true
		}
	}
	return false
}
