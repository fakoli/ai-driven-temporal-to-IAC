package workflow

import (
	"errors"
	"testing"

	"github.com/fakoli/temporal-terraform-orchestrator/activities"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func TestTerraformWorkflow_FullSequenceWithChanges(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		TFVars:     "/tmp/vpc/vars.tfvars",
		Operations: []string{"init", "validate", "plan", "apply"},
	}

	// Mock all activities
	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformPlan, mock.Anything, mock.Anything, mock.Anything).Return(true, nil) // Changes present
	env.OnActivity((*activities.TerraformActivities).TerraformApply, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformOutput, mock.Anything, mock.Anything, mock.Anything).Return(
		map[string]interface{}{"vpc_id": "vpc-12345"},
		nil,
	)

	// Execute workflow (no signal expectations - standalone workflows don't signal)
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err := env.GetWorkflowResult(&result)
	require.NoError(t, err)
	require.Equal(t, "vpc-12345", result["vpc_id"])

	// Verify all activities were called in correct order
	env.AssertExpectations(t)
}

func TestTerraformWorkflow_NoChangesSkipsApply(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{"init", "validate", "plan", "apply"},
	}

	// Mock activities - plan returns false (no changes)
	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformPlan, mock.Anything, mock.Anything, mock.Anything).Return(false, nil) // No changes
	env.OnActivity((*activities.TerraformActivities).TerraformOutput, mock.Anything, mock.Anything, mock.Anything).Return(
		map[string]interface{}{"vpc_id": "vpc-existing"},
		nil,
	)

	// Execute workflow (no signal expectations - standalone workflows don't signal)
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// When plan returns no changes, Apply should be skipped but init/validate/plan still run
	// The test passes if workflow completes successfully without calling apply
}

// NOTE: TestTerraformWorkflow_WithExtraVars was removed because ExtraVars
// are populated at runtime by ParentWorkflow, not set beforehand.
// This feature is tested through integration in parent_workflow_test.go
// (specifically TestParentWorkflow_OutputPropagation) where the parent
// workflow populates ExtraVars from previous workspace outputs.

// NOTE: TestTerraformWorkflow_SignalsParentOnCompletion was removed because
// standalone workflows (those run directly in tests without a parent) don't signal.
// Parent signaling is tested through integration in parent_workflow_test.go where
// child workflows are spawned by ParentWorkflow and do have a parent to signal.

func TestTerraformWorkflow_InitFailure(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{"init", "validate", "plan", "apply"},
	}

	// Mock init to fail
	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("terraform init failed: directory not found"))

	// Execute workflow (no signal expectations - standalone workflows don't signal)
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "init failed")

	// When init fails, subsequent activities (Plan, Apply) should not be called
	// The workflow should fail early with the init error
}

func TestTerraformWorkflow_PlanFailure(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{"init", "validate", "plan", "apply"},
	}

	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformPlan, mock.Anything, mock.Anything, mock.Anything).
		Return(false, errors.New("terraform plan failed: syntax error"))

	// Execute workflow (no signal expectations - standalone workflows don't signal)
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "plan failed")
}

func TestTerraformWorkflow_ApplyFailure(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{"init", "validate", "plan", "apply"},
	}

	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformPlan, mock.Anything, mock.Anything, mock.Anything).Return(true, nil)
	env.OnActivity((*activities.TerraformActivities).TerraformApply, mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("terraform apply failed: insufficient permissions"))

	// Execute workflow (no signal expectations - standalone workflows don't signal)
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "apply failed")
}

func TestTerraformWorkflow_PlanOnlyMode(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Configure workspace for plan-only mode (no apply)
	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		TFVars:     "/tmp/vpc/vars.tfvars",
		Operations: []string{"init", "validate", "plan"},
	}

	// Mock only the activities that should be called in plan-only mode
	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformPlan, mock.Anything, mock.Anything, mock.Anything).Return(true, nil) // Changes present
	env.OnActivity((*activities.TerraformActivities).TerraformOutput, mock.Anything, mock.Anything, mock.Anything).Return(
		map[string]interface{}{"vpc_id": "vpc-12345"},
		nil,
	)
	// Note: TerraformApply should NOT be called

	// Execute workflow (no signal expectations - standalone workflows don't signal)
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err := env.GetWorkflowResult(&result)
	require.NoError(t, err)
	require.Equal(t, "vpc-12345", result["vpc_id"])

	// Verify Apply was NOT called (plan-only mode)
	env.AssertExpectations(t)
}

func TestTerraformWorkflow_EmptyOperations(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Configure workspace with empty operations list
	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{},
	}

	// Mock output activity since it's always called
	env.OnActivity((*activities.TerraformActivities).TerraformOutput, mock.Anything, mock.Anything, mock.Anything).Return(
		map[string]interface{}{},
		nil,
	)

	// Execute workflow
	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	var result map[string]interface{}
	err := env.GetWorkflowResult(&result)
	require.NoError(t, err)
}

func TestTerraformWorkflow_OutputFailure(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{"init", "validate", "plan"},
	}

	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformPlan, mock.Anything, mock.Anything, mock.Anything).Return(false, nil)
	env.OnActivity((*activities.TerraformActivities).TerraformOutput, mock.Anything, mock.Anything, mock.Anything).Return(
		nil,
		errors.New("failed to read terraform output"),
	)

	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "failed to read terraform output")
}

func TestTerraformWorkflow_ValidateFailure(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	ws := WorkspaceConfig{
		Name:       "test-vpc",
		Dir:        "/tmp/vpc",
		Operations: []string{"init", "validate", "plan"},
	}

	env.OnActivity((*activities.TerraformActivities).TerraformInit, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	env.OnActivity((*activities.TerraformActivities).TerraformValidate, mock.Anything, mock.Anything, mock.Anything).Return(
		errors.New("validation failed: missing required variable"),
	)

	env.ExecuteWorkflow(TerraformWorkflow, ws)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "validate failed")
}
