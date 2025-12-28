// Package activities provides Temporal activities for executing Terraform operations.
// This file adds validation activities for pre-flight tfvars validation using CEL rules.
package activities

import (
	"context"
	"fmt"

	"github.com/fakoli/temporal-terraform-orchestrator/validation"
)

// ValidationActivities provides Temporal activities for tfvars validation
type ValidationActivities struct {
	service *validation.Service
}

// ValidateTFVarsParams contains parameters for the ValidateTFVars activity
type ValidateTFVarsParams struct {
	TFVarsPath    string                 // Path to combined tfvars JSON file
	TFVars        map[string]interface{} // Or direct tfvars map (used if TFVarsPath is empty)
	WorkspaceName string                 // Name of the workspace being validated
	WorkspaceKind string                 // Kind of workspace (terraform, tofu)
	WorkspaceDir  string                 // Directory of the workspace
	RulesPath     string                 // Optional: custom rules path
}

// ValidateTFVarsResult contains the result of tfvars validation
type ValidateTFVarsResult struct {
	Valid       bool                       `json:"valid"`
	Errors      []validation.ValidationIssue `json:"errors"`
	Warnings    []validation.ValidationIssue `json:"warnings"`
	ErrorCount  int                        `json:"error_count"`
	WarningCount int                       `json:"warning_count"`
	Summary     string                     `json:"summary"`
}

// NewValidationActivities creates a new ValidationActivities instance
func NewValidationActivities(rulesPath string) (*ValidationActivities, error) {
	svc, err := validation.NewService(rulesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create validation service: %w", err)
	}

	return &ValidationActivities{
		service: svc,
	}, nil
}

// NewValidationActivitiesWithService creates a ValidationActivities with an existing service
func NewValidationActivitiesWithService(svc *validation.Service) *ValidationActivities {
	return &ValidationActivities{
		service: svc,
	}
}

// ValidateTFVars validates tfvars against CEL rules before Terraform execution
func (a *ValidationActivities) ValidateTFVars(ctx context.Context, params ValidateTFVarsParams) (ValidateTFVarsResult, error) {
	result := ValidateTFVarsResult{
		Valid:    true,
		Errors:   []validation.ValidationIssue{},
		Warnings: []validation.ValidationIssue{},
	}

	// Determine which tfvars to use
	var tfvars map[string]interface{}
	var err error

	if params.TFVarsPath != "" {
		// Load from file
		tfvars, err = validation.LoadTFVarsJSON(params.TFVarsPath)
		if err != nil {
			return ValidateTFVarsResult{
				Valid: false,
				Errors: []validation.ValidationIssue{
					{
						Message:  fmt.Sprintf("Failed to load tfvars: %v", err),
						Severity: validation.SeverityError,
					},
				},
				ErrorCount: 1,
				Summary:    "Failed to load tfvars file",
			}, nil
		}
	} else if params.TFVars != nil {
		tfvars = params.TFVars
	} else {
		// No tfvars to validate - pass
		result.Summary = "No tfvars to validate"
		return result, nil
	}

	// Create workspace context
	wsCtx := validation.WorkspaceContext{
		Name: params.WorkspaceName,
		Kind: params.WorkspaceKind,
		Dir:  params.WorkspaceDir,
	}
	if wsCtx.Kind == "" {
		wsCtx.Kind = "terraform"
	}

	// If a custom rules path was provided, create a new service
	var svc *validation.Service
	if params.RulesPath != "" && params.RulesPath != validation.DefaultRulesPath {
		svc, err = validation.NewService(params.RulesPath)
		if err != nil {
			return ValidateTFVarsResult{
				Valid: false,
				Errors: []validation.ValidationIssue{
					{
						Message:  fmt.Sprintf("Failed to load rules: %v", err),
						Severity: validation.SeverityError,
					},
				},
				ErrorCount: 1,
				Summary:    "Failed to load validation rules",
			}, nil
		}
	} else {
		svc = a.service
	}

	// Run validation
	validationResult := svc.ValidateTFVars(tfvars, wsCtx)

	// Convert to activity result
	result.Valid = validationResult.Valid
	result.Errors = validationResult.Errors
	result.Warnings = validationResult.Warnings
	result.ErrorCount = len(validationResult.Errors)
	result.WarningCount = len(validationResult.Warnings)

	if result.Valid {
		if result.WarningCount > 0 {
			result.Summary = fmt.Sprintf("Validation passed with %d warning(s)", result.WarningCount)
		} else {
			result.Summary = "Validation passed"
		}
	} else {
		result.Summary = fmt.Sprintf("Validation failed with %d error(s) and %d warning(s)",
			result.ErrorCount, result.WarningCount)
	}

	return result, nil
}

// GetValidationService returns the underlying validation service
func (a *ValidationActivities) GetValidationService() *validation.Service {
	return a.service
}

// ReloadRules reloads the validation rules from disk
func (a *ValidationActivities) ReloadRules(ctx context.Context) error {
	return a.service.ReloadRules()
}

// FormatValidationError formats validation errors for human-readable output
func FormatValidationError(result ValidateTFVarsResult) string {
	if result.Valid {
		return result.Summary
	}

	msg := result.Summary + "\n\n"

	for i, err := range result.Errors {
		msg += fmt.Sprintf("%d. [%s] %s\n", i+1, err.Variable, err.Message)
		if err.Value != nil {
			msg += fmt.Sprintf("   Value: %v\n", err.Value)
		}
		if err.Remediation != "" {
			msg += fmt.Sprintf("   Fix: %s\n", err.Remediation)
		}
		msg += "\n"
	}

	return msg
}
