package activities

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestTerraformPlanDetectsChangesAndCreatesPlan(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir:      tmp,
		PlanFile: "tfplan-test.plan",
	}

	act := &TerraformActivities{}
	changed, err := act.TerraformPlan(context.Background(), params)
	require.NoError(t, err)
	require.True(t, changed, "plan should report changes when terraform exits 2")

	planPath := filepath.Join(tmp, params.PlanFile)
	_, statErr := os.Stat(planPath)
	require.NoError(t, statErr, "plan file should be created")
}

func TestTerraformApplyFailsWithoutPlan(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir:      tmp,
		PlanFile: "missing.plan",
	}
	act := &TerraformActivities{}

	err := act.TerraformApply(context.Background(), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plan file not found")
}

func TestTerraformValidateChecksConfiguration(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir: tmp,
	}

	act := &TerraformActivities{}
	err := act.TerraformValidate(context.Background(), params)
	require.NoError(t, err)
}

func TestTerraformInitRequiresValidDir(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	act := &TerraformActivities{}
	err := act.TerraformInit(context.Background(), TerraformParams{Dir: "/tmp/does-not-exist"})
	require.Error(t, err)
}

func TestTerraformOutput(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{Dir: tmp}

	act := &TerraformActivities{}
	outputs, err := act.TerraformOutput(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, outputs)
	require.Equal(t, "example-vpc-id", outputs["vpc_id"])
}

func TestTerraformPlanWithVars(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir:  tmp,
		Vars: map[string]interface{}{"foo": "bar"},
	}

	act := &TerraformActivities{}
	_, err := act.TerraformPlan(context.Background(), params)
	require.NoError(t, err)
}

func TestTerraformPlanWithTFVarsFile(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	tfvarsPath := filepath.Join(tmp, "test.tfvars")
	require.NoError(t, os.WriteFile(tfvarsPath, []byte("region = \"us-west-2\""), 0o644))

	params := TerraformParams{
		Dir:    tmp,
		TFVars: tfvarsPath,
	}

	act := &TerraformActivities{}
	_, err := act.TerraformPlan(context.Background(), params)
	require.NoError(t, err)
}

func TestTerraformPlanWithInvalidTFVarsFile(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir:    tmp,
		TFVars: "/nonexistent/vars.tfvars",
	}

	act := &TerraformActivities{}
	_, err := act.TerraformPlan(context.Background(), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "tfvars file invalid")
}

func TestTerraformValidateRequiresValidDir(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	params := TerraformParams{
		Dir: "/nonexistent/directory",
	}

	act := &TerraformActivities{}
	err := act.TerraformValidate(context.Background(), params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dir invalid")
}

func TestTerraformApplyWithValidPlan(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	planPath := filepath.Join(tmp, "test.plan")
	require.NoError(t, os.WriteFile(planPath, []byte("dummy plan"), 0o644))

	params := TerraformParams{
		Dir:      tmp,
		PlanFile: "test.plan",
	}

	act := &TerraformActivities{}
	err := act.TerraformApply(context.Background(), params)
	require.NoError(t, err)
}

func TestTerraformOutputWithEmptyResult(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPathWithEmptyOutput(t))

	tmp := t.TempDir()
	params := TerraformParams{Dir: tmp}

	act := &TerraformActivities{}
	outputs, err := act.TerraformOutput(context.Background(), params)
	require.NoError(t, err)
	require.NotNil(t, outputs)
	require.Equal(t, 0, len(outputs))
}

func TestValidatePathsWithEmptyDir(t *testing.T) {
	params := TerraformParams{
		Dir: "",
	}

	err := validatePaths(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dir is required")
}

func TestValidatePathsWithNonExistentDir(t *testing.T) {
	params := TerraformParams{
		Dir: "/nonexistent/directory",
	}

	err := validatePaths(params)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dir invalid")
}

func TestPlanFilePathWithCustomName(t *testing.T) {
	params := TerraformParams{
		Dir:      "/tmp/test",
		PlanFile: "custom-plan.tfplan",
	}

	path := planFilePath(params)
	require.Equal(t, "custom-plan.tfplan", path)
}

func TestPlanFilePathWithEmptyName(t *testing.T) {
	params := TerraformParams{
		Dir:      "/tmp/test",
		PlanFile: "",
	}

	path := planFilePath(params)
	require.Equal(t, "tfplan", path)
}

func TestPlanFullPathCombinesDirAndFile(t *testing.T) {
	params := TerraformParams{
		Dir:      "/tmp/test",
		PlanFile: "myplan.plan",
	}

	fullPath := planFullPath(params)
	require.Equal(t, "/tmp/test/myplan.plan", fullPath)
}

func TestIsJSONDetectsValidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"object", `{"key": "value"}`, true},
		{"array", `["item1", "item2"]`, true},
		{"with whitespace", `  {"key": "value"}  `, true},
		{"empty object", `{}`, true},
		{"empty array", `[]`, true},
		{"not json", `plain text`, false},
		{"empty", ``, false},
		{"xml", `<root></root>`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJSON([]byte(tt.input))
			require.Equal(t, tt.want, result)
		})
	}
}

// fakeTerraformOnPathWithEmptyOutput creates a terraform binary that returns empty output
func fakeTerraformOnPathWithEmptyOutput(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "terraform")
	script := `#!/bin/sh
cmd="$1"; shift
case "$cmd" in
  output)
    echo '{}'
    exit 0
    ;;
  *)
    exit 0
    ;;
esac
`
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return dir
}

// fakeTerraformOnPath creates a shim terraform binary that simulates Terraform
// CLI behavior for tests without touching real infrastructure.
func fakeTerraformOnPath(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	bin := filepath.Join(dir, "terraform")
	script := `#!/bin/sh
cmd="$1"; shift
case "$cmd" in
  init)
    exit 0
    ;;
  validate)
    exit 0
    ;;
  plan)
    out=""
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -out)
          out="$2"
          shift 2
          continue
          ;;
        -out=*)
          out=$(echo "$1" | sed 's/^-out=//')
          shift
          continue
          ;;
      esac
      shift
    done
    if [ -z "$out" ]; then
      out="tfplan"
    fi
    [ -n "$out" ] && touch "$out"
    exit 2
    ;;
  show)
    echo '{"format_version":"1.0"}'
    exit 0
    ;;
  apply)
    # Skip flags to find the plan file
    plan=""
    while [ "$#" -gt 0 ]; do
      case "$1" in
        -*)
          shift
          ;;
        *)
          plan="$1"
          break
          ;;
      esac
    done
    if [ -z "$plan" ] || [ ! -f "$plan" ]; then
      echo "missing plan" >&2
      exit 1
    fi
    exit 0
    ;;
  output)
    echo '{"vpc_id":{"value":"example-vpc-id"}}'
    exit 0
    ;;
  *)
    echo "unknown command" >&2
    exit 1
    ;;
esac
`
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	return dir
}

func TestCreateCombinedTFVars_NoExtraVars(t *testing.T) {
	params := TerraformParams{
		TFVars: "/path/to/original.tfvars",
		Vars:   map[string]interface{}{},
		RunID:  "test-run-id",
	}

	result, err := createCombinedTFVars(params)
	require.NoError(t, err)
	require.Equal(t, "/path/to/original.tfvars", result)
}

func TestCreateCombinedTFVars_HCLInputWithOverride(t *testing.T) {
	tmpDir := t.TempDir()
	originalTFVars := filepath.Join(tmpDir, "original.tfvars")

	// Create original HCL tfvars file with variables that will be overridden
	originalContent := `region = "us-west-2"
vpc_id = "vpc-original"
instance_type = "t2.micro"
`
	require.NoError(t, os.WriteFile(originalTFVars, []byte(originalContent), 0644))

	params := TerraformParams{
		TFVars: originalTFVars,
		Vars: map[string]interface{}{
			"vpc_id":     "vpc-from-parent",
			"subnet_ids": "subnet-123,subnet-456",
		},
		RunID: "test-hcl-run",
	}

	combinedPath, err := createCombinedTFVars(params)
	require.NoError(t, err)
	require.NotEmpty(t, combinedPath)

	// Verify it's a JSON file
	require.Contains(t, combinedPath, ".tfvars.json")

	// Read and parse JSON content
	content, err := os.ReadFile(combinedPath)
	require.NoError(t, err)

	var variables map[string]interface{}
	err = json.Unmarshal(content, &variables)
	require.NoError(t, err)

	// Should keep original vars that aren't overridden
	require.Equal(t, "us-west-2", variables["region"])
	require.Equal(t, "t2.micro", variables["instance_type"])

	// Should have the overridden vpc_id value, not the original
	require.Equal(t, "vpc-from-parent", variables["vpc_id"])

	// Should have the new variable
	require.Equal(t, "subnet-123,subnet-456", variables["subnet_ids"])
}

func TestCreateCombinedTFVars_JSONInput(t *testing.T) {
	tmpDir := t.TempDir()
	originalTFVars := filepath.Join(tmpDir, "original.tfvars.json")

	// Create original JSON tfvars file
	originalContent := `{
  "region": "us-east-1",
  "vpc_id": "vpc-original",
  "tags": {
    "Environment": "dev"
  }
}`
	require.NoError(t, os.WriteFile(originalTFVars, []byte(originalContent), 0644))

	params := TerraformParams{
		TFVars: originalTFVars,
		Vars: map[string]interface{}{
			"vpc_id":      "vpc-from-parent",
			"environment": "production",
		},
		RunID: "test-json-run",
	}

	combinedPath, err := createCombinedTFVars(params)
	require.NoError(t, err)
	require.NotEmpty(t, combinedPath)

	// Read and parse JSON content
	content, err := os.ReadFile(combinedPath)
	require.NoError(t, err)

	var variables map[string]interface{}
	err = json.Unmarshal(content, &variables)
	require.NoError(t, err)

	// Should keep original vars that aren't overridden
	require.Equal(t, "us-east-1", variables["region"])
	require.NotNil(t, variables["tags"])

	// Should have the overridden vpc_id value
	require.Equal(t, "vpc-from-parent", variables["vpc_id"])

	// Should have the new variable
	require.Equal(t, "production", variables["environment"])
}

func TestCreateCombinedTFVars_OnlyExtraVars(t *testing.T) {
	params := TerraformParams{
		TFVars: "",
		Vars: map[string]interface{}{
			"vpc_id":      "vpc-12345",
			"environment": "production",
		},
		RunID: "test-only-extra",
	}

	combinedPath, err := createCombinedTFVars(params)
	require.NoError(t, err)
	require.NotEmpty(t, combinedPath)

	// Read and parse JSON content
	content, err := os.ReadFile(combinedPath)
	require.NoError(t, err)

	var variables map[string]interface{}
	err = json.Unmarshal(content, &variables)
	require.NoError(t, err)

	require.Equal(t, "vpc-12345", variables["vpc_id"])
	require.Equal(t, "production", variables["environment"])
}

func TestCreateCombinedTFVars_HCLWithComplexTypes(t *testing.T) {
	tmpDir := t.TempDir()
	originalTFVars := filepath.Join(tmpDir, "original.tfvars")

	// Create HCL tfvars with complex types (lists, maps)
	originalContent := `region = "us-west-2"
availability_zones = ["us-west-2a", "us-west-2b"]
tags = {
  Environment = "dev"
  Project = "test"
}
`
	require.NoError(t, os.WriteFile(originalTFVars, []byte(originalContent), 0644))

	params := TerraformParams{
		TFVars: originalTFVars,
		Vars: map[string]interface{}{
			"region": "us-east-1", // Override region
		},
		RunID: "test-complex-types",
	}

	combinedPath, err := createCombinedTFVars(params)
	require.NoError(t, err)
	require.NotEmpty(t, combinedPath)

	// Read and parse JSON content
	content, err := os.ReadFile(combinedPath)
	require.NoError(t, err)

	var variables map[string]interface{}
	err = json.Unmarshal(content, &variables)
	require.NoError(t, err)

	// Region should be overridden
	require.Equal(t, "us-east-1", variables["region"])

	// Complex types should be preserved
	require.NotNil(t, variables["availability_zones"])
	require.NotNil(t, variables["tags"])
}

func TestCreateCombinedTFVars_ArrayFromParentWorkspace(t *testing.T) {
	// This test simulates the exact scenario from the user:
	// subnet workspace outputs subnet_ids as a JSON array
	// eks workspace should receive it as an array, not a string
	tmpDir := t.TempDir()
	originalTFVars := filepath.Join(tmpDir, "original.tfvars")

	// Create HCL tfvars with some existing variables
	originalContent := `region = "us-west-2"
instance_type = "t3.medium"
`
	require.NoError(t, os.WriteFile(originalTFVars, []byte(originalContent), 0644))

	// Simulate subnet_ids coming from subnet workspace output as a JSON array
	subnetIdsArray := []interface{}{"example-subnet-a", "example-subnet-b"}
	vpcIdString := "example-vpc-id"

	params := TerraformParams{
		TFVars: originalTFVars,
		Vars: map[string]interface{}{
			"vpc_id":     vpcIdString,
			"subnet_ids": subnetIdsArray, // This is a JSON array, not a string
		},
		RunID: "test-array-from-parent",
	}

	combinedPath, err := createCombinedTFVars(params)
	require.NoError(t, err)
	require.NotEmpty(t, combinedPath)

	// Read and parse JSON content
	content, err := os.ReadFile(combinedPath)
	require.NoError(t, err)

	var variables map[string]interface{}
	err = json.Unmarshal(content, &variables)
	require.NoError(t, err)

	// Verify original variables are preserved
	require.Equal(t, "us-west-2", variables["region"])
	require.Equal(t, "t3.medium", variables["instance_type"])

	// Verify vpc_id is a string
	require.Equal(t, "example-vpc-id", variables["vpc_id"])

	// Verify subnet_ids is preserved as an array, not converted to a string
	subnetIds, ok := variables["subnet_ids"].([]interface{})
	require.True(t, ok, "subnet_ids should be an array, not a string")
	require.Equal(t, 2, len(subnetIds))
	require.Equal(t, "example-subnet-a", subnetIds[0])
	require.Equal(t, "example-subnet-b", subnetIds[1])
}

func TestTerraformInit_ValidDirectory(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir: tmp,
	}

	act := &TerraformActivities{}
	err := act.TerraformInit(context.Background(), params)
	require.NoError(t, err)
}

func TestCtyToGo_NullValue(t *testing.T) {
	val := cty.NullVal(cty.String)
	result, err := ctyToGo(val)
	require.NoError(t, err)
	require.Nil(t, result)
}

func TestCtyToGo_NumberType(t *testing.T) {
	val := cty.NumberIntVal(42)
	result, err := ctyToGo(val)
	require.NoError(t, err)
	require.Equal(t, float64(42), result)
}

func TestCtyToGo_BoolType(t *testing.T) {
	val := cty.BoolVal(true)
	result, err := ctyToGo(val)
	require.NoError(t, err)
	require.Equal(t, true, result)
}

func TestCtyToGo_SetType(t *testing.T) {
	val := cty.SetVal([]cty.Value{
		cty.StringVal("item1"),
		cty.StringVal("item2"),
	})
	result, err := ctyToGo(val)
	require.NoError(t, err)
	require.IsType(t, []interface{}{}, result)
	arr := result.([]interface{})
	require.Equal(t, 2, len(arr))
}

func TestCtyToGo_MapType(t *testing.T) {
	val := cty.MapVal(map[string]cty.Value{
		"key1": cty.StringVal("value1"),
		"key2": cty.StringVal("value2"),
	})
	result, err := ctyToGo(val)
	require.NoError(t, err)
	require.IsType(t, map[string]interface{}{}, result)
	m := result.(map[string]interface{})
	require.Equal(t, "value1", m["key1"])
	require.Equal(t, "value2", m["key2"])
}

func TestTerraformPlan_DetailedExitCode(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	params := TerraformParams{
		Dir:      tmp,
		PlanFile: "test.plan",
	}

	act := &TerraformActivities{}
	changed, err := act.TerraformPlan(context.Background(), params)
	require.NoError(t, err)
	require.True(t, changed, "plan should detect changes when exit code is 2")
}

func TestRunTerraform_Success(t *testing.T) {
	t.Setenv("PATH", fakeTerraformOnPath(t))

	tmp := t.TempDir()
	err := runTerraform(context.Background(), tmp, "init")
	require.NoError(t, err)
}
