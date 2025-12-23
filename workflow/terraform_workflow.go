package workflow

import (
	"fmt"
	"time"

	"github.com/fakoli/temporal-terraform-orchestrator/activities"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func TerraformWorkflow(ctx workflow.Context, ws WorkspaceConfig) (map[string]interface{}, error) {

	options := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts:        3,
			InitialInterval:        5 * time.Second,
			BackoffCoefficient:     2.0,
			MaximumInterval:        1 * time.Minute,
			NonRetryableErrorTypes: []string{},
		},
	}
	ctx = workflow.WithActivityOptions(ctx, options)

	var a *activities.TerraformActivities
	info := workflow.GetInfo(ctx)
	rootRunID := info.WorkflowExecution.RunID
	if info.RootWorkflowExecution != nil {
		rootRunID = info.RootWorkflowExecution.RunID
	}

	planFile := fmt.Sprintf("tfplan-%s-%s.plan", info.WorkflowExecution.RunID, ws.Name)
	params := activities.TerraformParams{
		Dir:      ws.Dir,
		TFVars:   ws.TFVars,
		PlanFile: planFile,
		Vars:     ws.ExtraVars,
		RunID:    rootRunID,
	}

	// Determine orchestrator ID for signaling completion
	orchestratorID := ""
	if info.ParentWorkflowExecution != nil {
		orchestratorID = info.ParentWorkflowExecution.ID
	}
	if info.RootWorkflowExecution != nil {
		orchestratorID = info.RootWorkflowExecution.ID
	}

	signalParent := func(outs map[string]interface{}) {
		if orchestratorID == "" {
			// No parent workflow to signal (e.g., in test environment)
			return
		}
		finishedSignal := WorkspaceFinishedSignal{
			Name:    ws.Name,
			Outputs: outs,
		}
		if err := workflow.SignalExternalWorkflow(ctx, orchestratorID, "", SignalWorkspaceFinished, finishedSignal).Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("Failed to signal parent workflow", "workspace", ws.Name, "error", err)
		}
	}

	runTerraform := func() (map[string]interface{}, error) {
		changesPresent := false

		// Execute operations in the order specified
		for _, op := range ws.Operations {
			switch op {
			case "init":
				if err := workflow.ExecuteActivity(ctx, a.TerraformInit, params).Get(ctx, nil); err != nil {
					return nil, fmt.Errorf("init failed: %w", err)
				}

			case "validate":
				if err := workflow.ExecuteActivity(ctx, a.TerraformValidate, params).Get(ctx, nil); err != nil {
					return nil, fmt.Errorf("validate failed: %w", err)
				}

			case "plan":
				if err := workflow.ExecuteActivity(ctx, a.TerraformPlan, params).Get(ctx, &changesPresent); err != nil {
					return nil, fmt.Errorf("plan failed: %w", err)
				}
				if !changesPresent {
					workflow.GetLogger(ctx).Info("No changes detected in plan", "workspace", ws.Name, "dir", ws.Dir)
				}

			case "apply":
				// Only apply if there are changes
				if !changesPresent {
					workflow.GetLogger(ctx).Info("Skipping apply: no changes to apply", "workspace", ws.Name, "dir", ws.Dir)
					continue
				}
				if err := workflow.ExecuteActivity(ctx, a.TerraformApply, params).Get(ctx, nil); err != nil {
					return nil, fmt.Errorf("apply failed: %w", err)
				}

			default:
				return nil, fmt.Errorf("unknown operation: %s", op)
			}
		}

		// Always fetch outputs at the end (needed for dependent workspaces)
		var outputs map[string]interface{}
		err := workflow.ExecuteActivity(ctx, a.TerraformOutput, params).Get(ctx, &outputs)
		return outputs, err
	}

	// Execute Terraform operations
	outputs, err := runTerraform()
	signalParent(outputs)

	if err != nil {
		return nil, err
	}

	// Only enter hosting mode if this workflow has a parent (i.e., is part of an orchestration)
	if orchestratorID == "" {
		// Running standalone (e.g., in tests or direct execution) - exit immediately
		return outputs, nil
	}

	// Hosting mode: wait for child workflow signals or shutdown
	childChannel := workflow.GetSignalChannel(ctx, SignalStartChild)
	shutdownChannel := workflow.GetSignalChannel(ctx, SignalShutdown)
	activeChildren := 0
	shouldShutdown := false
	selector := workflow.NewSelector(ctx)

	selector.AddReceive(childChannel, func(c workflow.ReceiveChannel, more bool) {
		var signal StartChildSignal
		c.Receive(ctx, &signal)

		activeChildren++
		childOptions := workflow.ChildWorkflowOptions{
			WorkflowID: fmt.Sprintf("iac-%s-%s", rootRunID, signal.Workspace.Name),
		}
		if signal.Workspace.TaskQueue != "" {
			childOptions.TaskQueue = signal.Workspace.TaskQueue
		}

		ctxChild := workflow.WithChildOptions(ctx, childOptions)
		future := workflow.ExecuteChildWorkflow(ctxChild, TerraformWorkflow, signal.Workspace)

		// Add future to selector to track completion
		selector.AddFuture(future, func(f workflow.Future) {
			activeChildren--
			var childOut map[string]interface{}
			if err := f.Get(ctx, &childOut); err != nil {
				workflow.GetLogger(ctx).Error("Child workflow failed", "error", err)
			}
		})
	})

	selector.AddReceive(shutdownChannel, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, nil)
		shouldShutdown = true
	})

	for {
		selector.Select(ctx)
		if shouldShutdown && activeChildren == 0 {
			break
		}
	}

	return outputs, nil
}
