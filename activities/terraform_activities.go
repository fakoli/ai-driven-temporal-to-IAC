// Package activities provides Temporal activities for executing Terraform operations.
// It wraps the Terraform CLI to perform init, plan, validate, apply, and output commands
// with proper error handling and timeout management.
package activities

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

type TerraformParams struct {
	Dir      string
	TFVars   string
	PlanFile string
	Vars     map[string]interface{} // Preserves JSON types (string, array, object, etc.)
	RunID    string
}

type TerraformActivities struct{}

// createCombinedTFVars creates a combined tfvars file merging the original tfvars
// file with extra variables passed from parent workspaces. Extra vars override
// any variables with the same name in the original file.
// Uses HCL library for proper parsing and outputs as JSON for compatibility.
func createCombinedTFVars(params TerraformParams) (string, error) {
	// If no extra vars and no original tfvars, return empty
	if len(params.Vars) == 0 {
		return params.TFVars, nil
	}

	// Initialize variables map
	variables := make(map[string]interface{})

	// Parse original tfvars if provided
	if params.TFVars != "" {
		ext := filepath.Ext(params.TFVars)
		if ext == ".json" {
			// Parse as JSON
			data, err := os.ReadFile(params.TFVars)
			if err != nil {
				return "", fmt.Errorf("failed to read JSON tfvars file: %v", err)
			}
			if err := json.Unmarshal(data, &variables); err != nil {
				return "", fmt.Errorf("failed to parse JSON tfvars: %v", err)
			}
		} else {
			// Parse as HCL
			parser := hclparse.NewParser()
			var file *hcl.File
			var diags hcl.Diagnostics

			file, diags = parser.ParseHCLFile(params.TFVars)
			if diags.HasErrors() {
				return "", fmt.Errorf("failed to parse HCL tfvars: %v", diags.Error())
			}

			// Extract attributes from the HCL file
			attrs, diags := file.Body.JustAttributes()
			if diags.HasErrors() {
				return "", fmt.Errorf("failed to extract attributes from HCL: %v", diags.Error())
			}

			// Convert each attribute to a Go value
			for name, attr := range attrs {
				val, diags := attr.Expr.Value(nil)
				if diags.HasErrors() {
					return "", fmt.Errorf("failed to evaluate attribute %s: %v", name, diags.Error())
				}

				// Convert cty.Value to Go interface{}
				goValue, err := ctyToGo(val)
				if err != nil {
					return "", fmt.Errorf("failed to convert attribute %s: %v", name, err)
				}
				variables[name] = goValue
			}
		}
	}

	// Merge/override with extra vars from parent workspaces
	for key, value := range params.Vars {
		variables[key] = value
	}

	// Create temp directory for this run
	tmpDir := filepath.Join(os.TempDir(), "terraform-orchestrator", params.RunID)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temp directory: %v", err)
	}

	// Write as JSON (Terraform accepts .tfvars.json files)
	combinedPath := filepath.Join(tmpDir, "combined.tfvars.json")
	jsonData, err := json.MarshalIndent(variables, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal variables to JSON: %v", err)
	}

	if err := os.WriteFile(combinedPath, jsonData, 0644); err != nil {
		return "", fmt.Errorf("failed to write combined tfvars JSON: %v", err)
	}

	return combinedPath, nil
}

// ctyToGo converts a cty.Value to a Go interface{} for JSON serialization
func ctyToGo(val cty.Value) (interface{}, error) {
	if val.IsNull() {
		return nil, nil
	}

	valType := val.Type()

	switch {
	case valType == cty.String:
		return val.AsString(), nil
	case valType == cty.Number:
		var f float64
		if err := gocty.FromCtyValue(val, &f); err != nil {
			return nil, err
		}
		return f, nil
	case valType == cty.Bool:
		return val.True(), nil
	case valType.IsListType() || valType.IsSetType() || valType.IsTupleType():
		var result []interface{}
		it := val.ElementIterator()
		for it.Next() {
			_, elemVal := it.Element()
			elem, err := ctyToGo(elemVal)
			if err != nil {
				return nil, err
			}
			result = append(result, elem)
		}
		return result, nil
	case valType.IsMapType() || valType.IsObjectType():
		result := make(map[string]interface{})
		it := val.ElementIterator()
		for it.Next() {
			keyVal, elemVal := it.Element()
			key := keyVal.AsString()
			elem, err := ctyToGo(elemVal)
			if err != nil {
				return nil, err
			}
			result[key] = elem
		}
		return result, nil
	default:
		// For other types, try generic conversion
		var result interface{}
		if err := gocty.FromCtyValue(val, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
}

func (a *TerraformActivities) TerraformInit(ctx context.Context, params TerraformParams) error {
	if err := validatePaths(params); err != nil {
		return err
	}
	return runTerraform(ctx, params.Dir, "init")
}

func (a *TerraformActivities) TerraformPlan(ctx context.Context, params TerraformParams) (bool, error) {
	if err := validatePaths(params); err != nil {
		return false, err
	}

	// Create combined tfvars file if we have extra vars
	tfvarsFile, err := createCombinedTFVars(params)
	if err != nil {
		return false, err
	}

	planPath := planFullPath(params)
	args := []string{"plan", "-no-color", "-out", planPath, "-detailed-exitcode"}
	if tfvarsFile != "" {
		args = append(args, "-var-file", tfvarsFile)
	}

	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = params.Dir
	output, err := cmd.CombinedOutput()

	// Exit code 0: No changes, 2: Changes present
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 2 {
				if err := ensurePlanFile(planPath); err != nil {
					return false, fmt.Errorf("failed to create plan file: %v", err)
				}
				return true, nil // Changes present
			}
		}
		return false, fmt.Errorf("terraform plan failed: %v, args: %s, output: %s", err, strings.Join(args, " "), string(output))
	}

	if err := ensurePlanFile(planPath); err != nil {
		return false, fmt.Errorf("failed to create plan file: %v", err)
	}
	return false, nil // No changes
}

func (a *TerraformActivities) TerraformValidate(ctx context.Context, params TerraformParams) error {
	if err := validatePaths(params); err != nil {
		return err
	}
	return runTerraform(ctx, params.Dir, "validate")
}

func (a *TerraformActivities) TerraformApply(ctx context.Context, params TerraformParams) error {
	if err := validatePaths(params); err != nil {
		return err
	}
	planPath := planFullPath(params)

	if _, err := os.Stat(planPath); err != nil {
		return fmt.Errorf("plan file not found for apply: %s", planPath)
	}

	return runTerraform(ctx, params.Dir, "apply", "-no-color", planPath)
}

func (a *TerraformActivities) TerraformOutput(ctx context.Context, params TerraformParams) (map[string]interface{}, error) {
	if err := validatePaths(params); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, "terraform", "output", "-json")
	cmd.Dir = params.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("terraform output failed: %v, output: %s", err, string(output))
	}

	var raw map[string]struct {
		Value interface{} `json:"value"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse terraform output: %v", err)
	}

	results := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		results[k] = v.Value
	}
	return results, nil
}

func validatePaths(params TerraformParams) error {
	if strings.TrimSpace(params.Dir) == "" {
		return fmt.Errorf("terraform dir is required")
	}
	if info, err := os.Stat(params.Dir); err != nil || !info.IsDir() {
		return fmt.Errorf("terraform dir invalid: %v", err)
	}
	if params.TFVars != "" {
		if _, err := os.Stat(params.TFVars); err != nil {
			return fmt.Errorf("tfvars file invalid: %v", err)
		}
	}
	return nil
}

func runTerraform(ctx context.Context, dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "terraform", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("terraform %s failed: %v, output: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}

func planFilePath(params TerraformParams) string {
	if strings.TrimSpace(params.PlanFile) == "" {
		return "tfplan"
	}
	return filepath.Base(params.PlanFile)
}

func planFullPath(params TerraformParams) string {
	return filepath.Join(params.Dir, planFilePath(params))
}

func ensurePlanFile(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	// Only create the file if it doesn't exist; return error for other stat failures
	// (e.g., permission denied, path is a directory)
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat plan file %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
		return fmt.Errorf("failed to write plan file %s: %v", path, err)
	}
	return nil
}

func isJSON(data []byte) bool {
	data = []byte(strings.TrimSpace(string(data)))
	return len(data) > 0 && (data[0] == '{' || data[0] == '[')
}
