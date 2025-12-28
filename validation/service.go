package validation

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Service provides TFVars validation using CEL rules
type Service struct {
	rulesPath string
	ruleSet   *RuleSet
	engine    *CELEngine
}

// NewService creates a new validation service
func NewService(rulesPath string) (*Service, error) {
	if rulesPath == "" {
		rulesPath = DefaultRulesPath
	}

	// Create CEL engine
	engine, err := NewCELEngine()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL engine: %w", err)
	}

	// Load rules
	loader := NewRuleLoader(rulesPath)
	ruleSet, err := loader.LoadRules()
	if err != nil {
		return nil, fmt.Errorf("failed to load rules: %w", err)
	}

	return &Service{
		rulesPath: rulesPath,
		ruleSet:   ruleSet,
		engine:    engine,
	}, nil
}

// ValidateTFVars validates a tfvars map against all applicable rules
func (s *Service) ValidateTFVars(tfvars map[string]interface{}, wsCtx WorkspaceContext) ValidationResult {
	result := NewValidationResult()

	// Get applicable rules for this workspace
	rules := s.ruleSet.GetApplicableRules(wsCtx.Name)

	if len(rules) == 0 {
		// No rules to apply, validation passes
		return result
	}

	// Evaluate each rule
	for _, rule := range rules {
		issue := s.evaluateRule(rule, tfvars, wsCtx)
		if issue != nil {
			switch issue.Severity {
			case SeverityError:
				result.AddError(*issue)
			case SeverityWarning:
				result.AddWarning(*issue)
			case SeverityInfo:
				result.AddInfo(*issue)
			}
		}
	}

	return result
}

// ValidateTFVarsFile validates a tfvars JSON file
func (s *Service) ValidateTFVarsFile(tfvarsPath string, wsCtx WorkspaceContext) (ValidationResult, error) {
	// Load tfvars file
	tfvars, err := LoadTFVarsJSON(tfvarsPath)
	if err != nil {
		result := NewValidationResult()
		result.AddError(ValidationIssue{
			Message:  fmt.Sprintf("Failed to load tfvars file: %v", err),
			Severity: SeverityError,
		})
		return result, err
	}

	return s.ValidateTFVars(tfvars, wsCtx), nil
}

// ValidateRequest validates a full validation request
func (s *Service) ValidateRequest(req ValidationRequest) ValidationResult {
	return s.ValidateTFVars(req.TFVars, req.Workspace)
}

// evaluateRule evaluates a single rule and returns an issue if validation fails
func (s *Service) evaluateRule(rule *Rule, tfvars map[string]interface{}, wsCtx WorkspaceContext) *ValidationIssue {
	// Evaluate the CEL expression
	passed, err := s.engine.EvaluateRule(rule, tfvars, wsCtx)
	if err != nil {
		// Rule evaluation error is an error issue
		return &ValidationIssue{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			Message:  fmt.Sprintf("Rule evaluation error: %v", err),
			Severity: SeverityError,
			FilePath: rule.FilePath,
		}
	}

	if passed {
		return nil // Rule passed, no issue
	}

	// Rule failed - create issue
	issue := &ValidationIssue{
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		Message:     rule.Description,
		Severity:    rule.Severity,
		Remediation: rule.Remediation,
		FilePath:    rule.FilePath,
	}

	// Try to identify which variable(s) failed
	if len(rule.Target) > 0 {
		for _, target := range rule.Target {
			if val, ok := tfvars[target]; ok {
				issue.Variable = target
				issue.Value = val
				break // Use the first found target
			}
		}
		// If no specific target found, use the first one
		if issue.Variable == "" {
			issue.Variable = rule.Target[0]
		}
	}

	return issue
}

// ReloadRules reloads the rules from disk
func (s *Service) ReloadRules() error {
	loader := NewRuleLoader(s.rulesPath)
	ruleSet, err := loader.LoadRules()
	if err != nil {
		return err
	}
	s.ruleSet = ruleSet
	return nil
}

// GetRuleSet returns the current rule set
func (s *Service) GetRuleSet() *RuleSet {
	return s.ruleSet
}

// GetRuleCount returns the number of loaded rules
func (s *Service) GetRuleCount() int {
	return len(s.ruleSet.Rules)
}

// LoadTFVarsJSON loads and parses a JSON tfvars file
func LoadTFVarsJSON(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read tfvars file: %w", err)
	}

	var tfvars map[string]interface{}
	if err := json.Unmarshal(data, &tfvars); err != nil {
		return nil, fmt.Errorf("failed to parse tfvars JSON: %w", err)
	}

	return tfvars, nil
}

// ValidateMultipleWorkspaces validates multiple workspaces and returns a combined response
func (s *Service) ValidateMultipleWorkspaces(requests []ValidationRequest) ValidationResponse {
	response := ValidationResponse{
		Status:     "complete",
		Workspaces: make(map[string]ValidationResult),
		Summary: ValidationSummary{
			TotalWorkspaces: len(requests),
		},
	}

	for _, req := range requests {
		result := s.ValidateRequest(req)
		response.Workspaces[req.Workspace.Name] = result

		if result.Valid {
			response.Summary.ValidWorkspaces++
		} else {
			response.Summary.FailedWorkspaces++
			response.Status = "incomplete"
		}

		response.Summary.TotalErrors += len(result.Errors)
		response.Summary.TotalWarnings += len(result.Warnings)
	}

	return response
}

// FormatResponse formats a validation response as JSON
func (s *Service) FormatResponse(response ValidationResponse) (string, error) {
	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// FormatResultText formats a single validation result as human-readable text
func (s *Service) FormatResultText(workspaceName string, result ValidationResult) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("Workspace: %s\n", workspaceName))
	b.WriteString(result.FormatText())

	return b.String()
}

// QuickValidate is a convenience function for validating without creating a service instance
func QuickValidate(tfvars map[string]interface{}, workspaceName string, rulesPath string) (ValidationResult, error) {
	svc, err := NewService(rulesPath)
	if err != nil {
		return ValidationResult{}, err
	}

	wsCtx := WorkspaceContext{
		Name: workspaceName,
		Kind: "terraform",
	}

	return svc.ValidateTFVars(tfvars, wsCtx), nil
}

// QuickValidateFile is a convenience function for validating a file
func QuickValidateFile(tfvarsPath string, workspaceName string, rulesPath string) (ValidationResult, error) {
	tfvars, err := LoadTFVarsJSON(tfvarsPath)
	if err != nil {
		return ValidationResult{}, err
	}

	return QuickValidate(tfvars, workspaceName, rulesPath)
}
