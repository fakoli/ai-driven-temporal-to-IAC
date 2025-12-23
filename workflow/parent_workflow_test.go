package workflow

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestParentWorkflowExecutesChildrenInOrder(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	var executionOrder []string
	var mu sync.Mutex

	stubWF := func(ctx workflow.Context, ws WorkspaceConfig) (map[string]interface{}, error) {
		mu.Lock()
		executionOrder = append(executionOrder, ws.Name)
		mu.Unlock()

		env.SignalWorkflow(SignalWorkspaceFinished, WorkspaceFinishedSignal{
			Name:    ws.Name,
			Outputs: map[string]interface{}{"out": "val"},
		})

		return map[string]interface{}{"out": "val"}, nil
	}

	env.RegisterWorkflowWithOptions(stubWF, workflow.RegisterOptions{Name: "TerraformWorkflow"})

	// Mock signals to fail so fallback logic is triggered in tests
	env.OnSignalExternalWorkflow(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("fallback"))

	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "vpc", Dir: "/tmp/vpc"},
			{Name: "subnets", Dir: "/tmp/subnets", DependsOn: []string{"vpc"}},
			{Name: "eks", Dir: "/tmp/eks", DependsOn: []string{"vpc", "subnets"}},
		},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	require.Equal(t, 3, len(executionOrder))
	require.Equal(t, "vpc", executionOrder[0])
	require.Equal(t, "subnets", executionOrder[1])
	require.Equal(t, "eks", executionOrder[2])
}

func TestParentWorkflowDeepNesting(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	var executionOrder []string
	var mu sync.Mutex

	stubWF := func(ctx workflow.Context, ws WorkspaceConfig) (map[string]interface{}, error) {
		mu.Lock()
		executionOrder = append(executionOrder, ws.Name)
		mu.Unlock()

		env.SignalWorkflow(SignalWorkspaceFinished, WorkspaceFinishedSignal{
			Name:    ws.Name,
			Outputs: map[string]interface{}{"out": "val"},
		})
		return map[string]interface{}{"out": "val"}, nil
	}

	env.RegisterWorkflowWithOptions(stubWF, workflow.RegisterOptions{Name: "TerraformWorkflow"})

	env.OnSignalExternalWorkflow(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("fallback"))

	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "a", Dir: "/tmp/a"},
			{Name: "b", Dir: "/tmp/b", DependsOn: []string{"a"}},
			{Name: "c", Dir: "/tmp/c", DependsOn: []string{"a", "b"}},
		},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, 3, len(executionOrder))
	require.Equal(t, "a", executionOrder[0])
	require.Equal(t, "b", executionOrder[1])
	require.Equal(t, "c", executionOrder[2])
}

func TestParentWorkflow_ParallelExecution(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	var executionOrder []string
	var mu sync.Mutex

	stubWF := func(ctx workflow.Context, ws WorkspaceConfig) (map[string]interface{}, error) {
		mu.Lock()
		executionOrder = append(executionOrder, ws.Name)
		mu.Unlock()

		env.SignalWorkflow(SignalWorkspaceFinished, WorkspaceFinishedSignal{
			Name:    ws.Name,
			Outputs: map[string]interface{}{},
		})
		return map[string]interface{}{}, nil
	}

	env.RegisterWorkflowWithOptions(stubWF, workflow.RegisterOptions{Name: "TerraformWorkflow"})

	env.OnSignalExternalWorkflow(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("fallback"))

	// Three independent workspaces (no dependencies)
	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "vpc-1", Dir: "/tmp/vpc-1"},
			{Name: "vpc-2", Dir: "/tmp/vpc-2"},
			{Name: "vpc-3", Dir: "/tmp/vpc-3"},
		},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// All three should execute (order doesn't matter for parallel execution)
	require.Equal(t, 3, len(executionOrder))
}

func TestParentWorkflow_OutputPropagation(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	var capturedWorkspaces []WorkspaceConfig
	var mu sync.Mutex

	stubWF := func(ctx workflow.Context, ws WorkspaceConfig) (map[string]interface{}, error) {
		mu.Lock()
		capturedWorkspaces = append(capturedWorkspaces, ws)
		mu.Unlock()

		// VPC outputs vpc_id
		outputs := map[string]interface{}{}
		if ws.Name == "vpc" {
			outputs["vpc_id"] = "vpc-12345"
			outputs["vpc_cidr"] = "10.0.0.0/16"
		}

		env.SignalWorkflow(SignalWorkspaceFinished, WorkspaceFinishedSignal{
			Name:    ws.Name,
			Outputs: outputs,
		})
		return outputs, nil
	}

	env.RegisterWorkflowWithOptions(stubWF, workflow.RegisterOptions{Name: "TerraformWorkflow"})

	env.OnSignalExternalWorkflow(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("fallback"))

	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "vpc", Dir: "/tmp/vpc"},
			{
				Name:      "subnets",
				Dir:       "/tmp/subnets",
				DependsOn: []string{"vpc"},
				Inputs: []InputMapping{
					{SourceWorkspace: "vpc", SourceOutput: "vpc_id", TargetVar: "vpc_id"},
					{SourceWorkspace: "vpc", SourceOutput: "vpc_cidr", TargetVar: "cidr_block"},
				},
			},
		},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// Find the subnets workspace in captured workspaces
	var subnetsWS *WorkspaceConfig
	for i := range capturedWorkspaces {
		if capturedWorkspaces[i].Name == "subnets" {
			subnetsWS = &capturedWorkspaces[i]
			break
		}
	}

	require.NotNil(t, subnetsWS, "subnets workspace should have been executed")

	// Verify outputs were propagated to ExtraVars
	require.NotNil(t, subnetsWS.ExtraVars)
	require.Equal(t, "vpc-12345", subnetsWS.ExtraVars["vpc_id"])
	require.Equal(t, "10.0.0.0/16", subnetsWS.ExtraVars["cidr_block"])
}

func TestParentWorkflow_InvalidConfigAtRuntime(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	// Config with cycle
	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "a", Dir: "/tmp/a", DependsOn: []string{"b"}},
			{Name: "b", Dir: "/tmp/b", DependsOn: []string{"a"}},
		},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "cycle")
}

func TestParentWorkflow_EmptyWorkspaceList(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "no workspaces")
}

func TestParentWorkflow_ComplexDependencyGraph(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	var executionOrder []string
	var mu sync.Mutex

	stubWF := func(ctx workflow.Context, ws WorkspaceConfig) (map[string]interface{}, error) {
		mu.Lock()
		executionOrder = append(executionOrder, ws.Name)
		mu.Unlock()

		env.SignalWorkflow(SignalWorkspaceFinished, WorkspaceFinishedSignal{
			Name:    ws.Name,
			Outputs: map[string]interface{}{},
		})
		return map[string]interface{}{}, nil
	}

	// Register workflow
	env.RegisterWorkflowWithOptions(stubWF, workflow.RegisterOptions{Name: "TerraformWorkflow"})

	env.OnSignalExternalWorkflow(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("fallback"))

	// Complex DAG:
	// vpc (root)
	// ├── subnets (depends on vpc)
	// ├── db (depends on vpc)
	// └── eks (depends on vpc, subnets)
	//     └── app (depends on eks, db)
	cfg := InfrastructureConfig{
		Workspaces: []WorkspaceConfig{
			{Name: "vpc", Dir: "/tmp/vpc"},
			{Name: "subnets", Dir: "/tmp/subnets", DependsOn: []string{"vpc"}},
			{Name: "db", Dir: "/tmp/db", DependsOn: []string{"vpc"}},
			{Name: "eks", Dir: "/tmp/eks", DependsOn: []string{"vpc", "subnets"}},
			{Name: "app", Dir: "/tmp/app", DependsOn: []string{"eks", "db"}},
		},
	}

	env.ExecuteWorkflow(ParentWorkflow, cfg)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	require.Equal(t, 5, len(executionOrder))

	// Verify execution order respects dependencies
	vpcIdx := indexOf(executionOrder, "vpc")
	subnetsIdx := indexOf(executionOrder, "subnets")
	dbIdx := indexOf(executionOrder, "db")
	eksIdx := indexOf(executionOrder, "eks")
	appIdx := indexOf(executionOrder, "app")

	// vpc must come before all others
	require.True(t, vpcIdx < subnetsIdx)
	require.True(t, vpcIdx < dbIdx)
	require.True(t, vpcIdx < eksIdx)
	require.True(t, vpcIdx < appIdx)

	// subnets must come before eks
	require.True(t, subnetsIdx < eksIdx)

	// eks and db must come before app
	require.True(t, eksIdx < appIdx)
	require.True(t, dbIdx < appIdx)
}

// Helper function to find index of element in slice
func indexOf(slice []string, value string) int {
	for i, v := range slice {
		if v == value {
			return i
		}
	}
	return -1
}
