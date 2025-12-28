package validation

import (
	"fmt"
	"strings"
	"time"
)

// Severity represents the severity level of a validation issue
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// ValidationResult represents the result of validating tfvars for a workspace
type ValidationResult struct {
	Valid     bool              `json:"valid"`
	Errors    []ValidationIssue `json:"errors"`
	Warnings  []ValidationIssue `json:"warnings"`
	Info      []ValidationIssue `json:"info"`
	Timestamp time.Time         `json:"timestamp"`
}

// ValidationIssue represents a single validation error, warning, or info message
type ValidationIssue struct {
	RuleID      string      `json:"rule_id"`                // e.g., "vpc.rfc1918"
	RuleName    string      `json:"rule_name"`              // e.g., "RFC 1918 Validation"
	Variable    string      `json:"variable,omitempty"`     // e.g., "private_subnet"
	Value       interface{} `json:"value,omitempty"`        // Actual failing value
	Message     string      `json:"message"`                // Human-readable error
	Severity    Severity    `json:"severity"`               // error, warning, info
	Remediation string      `json:"remediation,omitempty"`  // Suggested fix
	FilePath    string      `json:"file_path,omitempty"`    // Rule file that triggered this
	Line        int         `json:"line,omitempty"`         // Line in tfvars file if applicable
}

// Rule represents a CEL validation rule loaded from a file
type Rule struct {
	ID          string   // Auto-generated from file path (e.g., "vpc.rfc1918")
	Name        string   // Parsed from filename (e.g., "rfc1918")
	FilePath    string   // Absolute path to rule file
	Category    string   // Parsed from directory (e.g., "vpc", "common")
	Target      []string // Variables this rule applies to
	Severity    Severity // error, warning, info
	Description string   // Human-readable description
	Remediation string   // Suggested fix when validation fails
	Workspace   string   // Workspace pattern to apply rule (e.g., "vpc", "*")
	Order       int      // Explicit ordering (0 = use default)
	Expression  string   // CEL expression text
}

// RuleSet represents a collection of loaded rules
type RuleSet struct {
	Rules       []Rule
	RulesPath   string
	LoadedAt    time.Time
	RulesByID   map[string]*Rule
	RulesByWS   map[string][]*Rule // Rules indexed by workspace
}

// WorkspaceContext provides context about the workspace being validated
type WorkspaceContext struct {
	Name string // Workspace name (e.g., "vpc", "eks")
	Kind string // Workspace kind (e.g., "terraform", "tofu")
	Dir  string // Workspace directory path
}

// ValidationRequest represents a request to validate tfvars
type ValidationRequest struct {
	TFVars        map[string]interface{} // Combined tfvars to validate
	Workspace     WorkspaceContext       // Workspace context
	RulesPath     string                 // Optional: custom rules path
	FailOnWarning bool                   // Treat warnings as errors
}

// ValidationResponse represents the complete validation response (for MCP)
type ValidationResponse struct {
	Status     string                       `json:"validation_status"` // "complete", "incomplete"
	Workspaces map[string]ValidationResult  `json:"workspaces"`
	Summary    ValidationSummary            `json:"summary"`
}

// ValidationSummary provides aggregate statistics
type ValidationSummary struct {
	TotalWorkspaces  int `json:"total_workspaces"`
	ValidWorkspaces  int `json:"valid_workspaces"`
	FailedWorkspaces int `json:"failed_workspaces"`
	TotalErrors      int `json:"total_errors"`
	TotalWarnings    int `json:"total_warnings"`
}

// NewValidationResult creates a new empty validation result
func NewValidationResult() ValidationResult {
	return ValidationResult{
		Valid:     true,
		Errors:    []ValidationIssue{},
		Warnings:  []ValidationIssue{},
		Info:      []ValidationIssue{},
		Timestamp: time.Now(),
	}
}

// AddError adds an error to the validation result
func (r *ValidationResult) AddError(issue ValidationIssue) {
	issue.Severity = SeverityError
	r.Errors = append(r.Errors, issue)
	r.Valid = false
}

// AddWarning adds a warning to the validation result
func (r *ValidationResult) AddWarning(issue ValidationIssue) {
	issue.Severity = SeverityWarning
	r.Warnings = append(r.Warnings, issue)
}

// AddInfo adds an info message to the validation result
func (r *ValidationResult) AddInfo(issue ValidationIssue) {
	issue.Severity = SeverityInfo
	r.Info = append(r.Info, issue)
}

// HasIssues returns true if there are any errors or warnings
func (r *ValidationResult) HasIssues() bool {
	return len(r.Errors) > 0 || len(r.Warnings) > 0
}

// FormatError returns a human-readable string of all errors
func (r *ValidationResult) FormatError() string {
	if r.Valid {
		return "Validation passed"
	}

	var b strings.Builder
	b.WriteString("Validation failed with the following errors:\n\n")

	for i, err := range r.Errors {
		b.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, err.Variable, err.Message))
		if err.Value != nil {
			b.WriteString(fmt.Sprintf("   Value: %v\n", err.Value))
		}
		if err.Remediation != "" {
			b.WriteString(fmt.Sprintf("   Fix: %s\n", err.Remediation))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// FormatText returns a complete human-readable summary
func (r *ValidationResult) FormatText() string {
	var b strings.Builder

	if r.Valid {
		b.WriteString("Status: PASSED\n")
	} else {
		b.WriteString("Status: FAILED\n")
	}

	if len(r.Errors) > 0 {
		b.WriteString("\nErrors:\n")
		for _, err := range r.Errors {
			b.WriteString(fmt.Sprintf("  • [%s] %s: %s\n", err.Variable, err.RuleName, err.Message))
			if err.Value != nil {
				b.WriteString(fmt.Sprintf("    Value: %v\n", err.Value))
			}
			if err.Remediation != "" {
				b.WriteString(fmt.Sprintf("    Fix: %s\n", err.Remediation))
			}
		}
	}

	if len(r.Warnings) > 0 {
		b.WriteString("\nWarnings:\n")
		for _, warn := range r.Warnings {
			b.WriteString(fmt.Sprintf("  • [%s] %s: %s\n", warn.Variable, warn.RuleName, warn.Message))
			if warn.Remediation != "" {
				b.WriteString(fmt.Sprintf("    Suggestion: %s\n", warn.Remediation))
			}
		}
	}

	if len(r.Info) > 0 {
		b.WriteString("\nInfo:\n")
		for _, info := range r.Info {
			b.WriteString(fmt.Sprintf("  • %s\n", info.Message))
		}
	}

	return b.String()
}

// Merge combines another ValidationResult into this one
func (r *ValidationResult) Merge(other ValidationResult) {
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
	r.Info = append(r.Info, other.Info...)
	if !other.Valid {
		r.Valid = false
	}
}

// GetRulesForWorkspace returns rules applicable to the given workspace
func (rs *RuleSet) GetRulesForWorkspace(workspaceName string) []*Rule {
	var rules []*Rule

	// First, add common rules (lowest precedence)
	if commonRules, ok := rs.RulesByWS["*"]; ok {
		rules = append(rules, commonRules...)
	}
	if commonRules, ok := rs.RulesByWS["common"]; ok {
		rules = append(rules, commonRules...)
	}

	// Then add workspace-specific rules (higher precedence)
	if wsRules, ok := rs.RulesByWS[workspaceName]; ok {
		rules = append(rules, wsRules...)
	}

	return rules
}
