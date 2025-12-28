# Test Coverage Analysis & Improvement Recommendations

## Current Coverage Summary

| Package | Coverage | Status |
|---------|----------|--------|
| `activities/` | 80.5% | Good |
| `workflow/` | 84.6% | Good |
| `cmd/mcp-server/` | 0.0% | Not tested |
| `cmd/starter/` | 0.0% | Not tested |
| `cmd/worker/` | 0.0% | Not tested |
| **Total** | 68.0% | Needs improvement |

## Detailed Coverage Breakdown

### Activities Package (80.5%)

| Function | Coverage | Notes |
|----------|----------|-------|
| `createCombinedTFVars` | 77.5% | Missing: HCL parse errors, JSON read failures |
| `ctyToGo` | 78.8% | Missing: generic conversion fallback |
| `TerraformInit` | 100.0% | Fully covered |
| `TerraformPlan` | 72.7% | Missing: non-2 exit code error paths |
| `TerraformValidate` | 100.0% | Fully covered |
| `TerraformApply` | 83.3% | Good coverage |
| `TerraformOutput` | 78.6% | Missing: JSON parse failures |
| `validatePaths` | 100.0% | Fully covered |
| `runTerraform` | 87.5% | Missing: timeout handling |
| `planFilePath` | 100.0% | Fully covered |
| `planFullPath` | 100.0% | Fully covered |
| `ensurePlanFile` | 66.7% | Missing: file write error path |
| `isJSON` | 100.0% | Fully covered |

### Workflow Package (84.6%)

| Function | Coverage | Notes |
|----------|----------|-------|
| `NormalizeInfrastructureConfig` | 95.0% | Missing: os.Getwd() error path |
| `ValidateInfrastructureConfig` | 96.0% | Excellent coverage |
| `ValidateWorkspaceOperations` | 87.5% | Missing: non-terraform kind error |
| `validateTerraformOperations` | 100.0% | Fully covered |
| `isTransitivelyDependent` | 100.0% | Fully covered |
| `CalculateDepths` | 100.0% | Fully covered |
| `isSupportedKind` | 100.0% | Fully covered |
| `getDefaultOperations` | 80.0% | Missing: empty string kind |
| `LoadConfigFromFile` | 92.3% | Missing: no-extension fallback error |
| `ParentWorkflow` | 89.8% | Good coverage |
| `isRunning` | 100.0% | Fully covered |
| `allDependenciesMet` | 100.0% | Fully covered |
| `startWorkspace` | 83.9% | Missing: some error paths |
| `TerraformWorkflow` | 54.1% | **Critical gap**: hosting mode untested |

---

## Critical Gaps to Address

### 1. MCP Server (cmd/mcp-server) - 0% Coverage - HIGH PRIORITY

The MCP server has 4 untested handlers:

- `listWorkflowsHandler` - Lists available workflows and workspaces
- `executeWorkflowHandler` - Starts a workflow execution
- `getWorkflowStatusHandler` - Gets workflow execution status
- `main()` - Server initialization

**Why critical:** The MCP server is the AI integration point. Bugs here could cause AI agents to fail silently or execute incorrect infrastructure changes.

**Recommended tests:**
```go
// cmd/mcp-server/handlers_test.go
func TestListWorkflowsHandler_ConfigFileNotFound(t *testing.T)
func TestListWorkflowsHandler_ValidConfig(t *testing.T)
func TestListWorkflowsHandler_InvalidConfig(t *testing.T)
func TestExecuteWorkflowHandler_UnsupportedWorkflow(t *testing.T)
func TestExecuteWorkflowHandler_MissingConfig(t *testing.T)
func TestExecuteWorkflowHandler_InlineConfig(t *testing.T)
func TestExecuteWorkflowHandler_ConfigPath(t *testing.T)
func TestGetWorkflowStatusHandler_WorkflowNotFound(t *testing.T)
func TestGetWorkflowStatusHandler_ValidWorkflow(t *testing.T)
```

### 2. TerraformWorkflow Hosting Mode - 54.1% Coverage - MEDIUM PRIORITY

Lines 119-166 of `workflow/terraform_workflow.go` are completely untested:

- Child workflow signal handling (`SignalStartChild`)
- Shutdown signal handling (`SignalShutdown`)
- Active children tracking
- Selector loop for child workflow completion

**Recommended tests:**
```go
func TestTerraformWorkflow_HostingModeWithParent(t *testing.T)
func TestTerraformWorkflow_ChildWorkflowSpawning(t *testing.T)
func TestTerraformWorkflow_ShutdownSignal(t *testing.T)
func TestTerraformWorkflow_MultipleChildWorkflows(t *testing.T)
func TestTerraformWorkflow_ChildWorkflowFailure(t *testing.T)
```

### 3. Error Path Coverage in Activities - MEDIUM PRIORITY

Several error paths lack test coverage:

```go
func TestCreateCombinedTFVars_InvalidHCLSyntax(t *testing.T)
func TestCreateCombinedTFVars_JSONReadFailure(t *testing.T)
func TestTerraformPlan_ExitCode1Error(t *testing.T)
func TestTerraformOutput_InvalidJSON(t *testing.T)
func TestRunTerraform_Timeout(t *testing.T)
```

### 4. Integration/End-to-End Tests - 0% - HIGH PRIORITY

Currently NO integration tests verify:
- Actual Temporal workflow execution
- Real Terraform binary interaction
- Multi-workspace orchestration with real files
- Output propagation between actual workspaces

**Recommended approach:**
```go
// integration_test.go (with build tag)
//go:build integration

func TestIntegration_SimpleWorkspaceExecution(t *testing.T)
func TestIntegration_MultiWorkspaceDependencies(t *testing.T)
func TestIntegration_OutputPropagation(t *testing.T)
```

---

## Prioritized Action Plan

### Phase 1: Critical Gaps (Immediate)

1. **Add MCP server handler tests** - Prevents AI integration bugs
2. **Add TerraformPlan error path tests** - Prevents silent failures on Terraform errors
3. **Add TerraformOutput parse failure tests** - Prevents output propagation bugs

### Phase 2: Important Coverage (Short-term)

4. **Add hosting mode tests for TerraformWorkflow** - Tests child workflow orchestration
5. **Add integration test framework** - Enables testing real Terraform execution
6. **Add createCombinedTFVars error tests** - Prevents variable handling bugs

### Phase 3: Comprehensive Coverage (Medium-term)

7. **Add CLI starter/worker basic tests** - Tests command-line entry points
8. **Add context timeout tests** - Ensures proper timeout handling
9. **Add concurrent execution stress tests** - Ensures thread safety

---

## Example Test Implementations

### MCP Server Handler Tests

```go
package main

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/mark3labs/mcp-go/mcp"
    "github.com/stretchr/testify/require"
)

func TestListWorkflowsHandler_MissingConfig(t *testing.T) {
    request := mcp.CallToolRequest{}
    // Set config_path to non-existent file

    result, err := listWorkflowsHandler(context.Background(), request)
    require.NoError(t, err)
    require.NotNil(t, result)
    // Verify it returns default schema info
}

func TestListWorkflowsHandler_ValidConfig(t *testing.T) {
    tmpDir := t.TempDir()
    configPath := filepath.Join(tmpDir, "infra.yaml")
    configContent := `
workspace_root: /tmp
workspaces:
  - name: vpc
    dir: vpc
`
    require.NoError(t, os.WriteFile(configPath, []byte(configContent), 0644))

    // Create request with config_path
    // Verify workspaces are returned
}
```

### TerraformWorkflow Hosting Mode Tests

```go
func TestTerraformWorkflow_WithParentSignaling(t *testing.T) {
    suite := &testsuite.WorkflowTestSuite{}
    env := suite.NewTestWorkflowEnvironment()

    // Set up parent workflow info
    // Test that workspace-finished signal is sent
    // Test hosting mode is entered when parent exists
}

func TestTerraformWorkflow_ShutdownSignal(t *testing.T) {
    suite := &testsuite.WorkflowTestSuite{}
    env := suite.NewTestWorkflowEnvironment()

    // Start workflow in hosting mode
    // Send shutdown signal
    // Verify graceful exit
}
```

### Error Path Tests

```go
func TestCreateCombinedTFVars_InvalidHCL(t *testing.T) {
    tmpDir := t.TempDir()
    badHCL := filepath.Join(tmpDir, "bad.tfvars")
    os.WriteFile(badHCL, []byte("this is { not valid hcl"), 0644)

    params := TerraformParams{
        TFVars: badHCL,
        Vars:   map[string]interface{}{"x": "y"},
        RunID:  "test",
    }

    _, err := createCombinedTFVars(params)
    require.Error(t, err)
    require.Contains(t, err.Error(), "parse HCL")
}

func TestTerraformOutput_InvalidJSON(t *testing.T) {
    // Create fake terraform that returns invalid JSON
    t.Setenv("PATH", fakeTerraformWithInvalidOutput(t))

    act := &TerraformActivities{}
    _, err := act.TerraformOutput(context.Background(), TerraformParams{Dir: t.TempDir()})
    require.Error(t, err)
    require.Contains(t, err.Error(), "parse terraform output")
}
```

---

## Expected Improvement

After implementing the recommended tests:

| Package | Current | Target |
|---------|---------|--------|
| `activities/` | 80.5% | 95%+ |
| `workflow/` | 84.6% | 95%+ |
| `cmd/mcp-server/` | 0.0% | 85%+ |
| `cmd/starter/` | 0.0% | 70%+ |
| `cmd/worker/` | 0.0% | 70%+ |
| **Total** | 68.0% | 85-90% |

---

## Testing Best Practices

1. **Use table-driven tests** for comprehensive input validation
2. **Mock Temporal activities** for workflow unit tests
3. **Use fake terraform binaries** for activity tests without real infrastructure
4. **Add build tags** for integration tests that require real Terraform/Temporal
5. **Test error messages** to ensure helpful debugging information
6. **Test concurrent access** to verify thread safety in signal handlers
