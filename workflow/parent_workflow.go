package workflow

import (
	"fmt"

	"go.temporal.io/sdk/workflow"
)

func ParentWorkflow(ctx workflow.Context, rawConfig InfrastructureConfig) error {
	if err := ValidateInfrastructureConfig(rawConfig); err != nil {
		return err
	}

	config := NormalizeInfrastructureConfig(rawConfig)
	workflow.GetLogger(ctx).Info("Starting parent workflow", "workspaces", len(config.Workspaces))

	depths := CalculateDepths(config.Workspaces)
	completedWorkspaces := make(map[string]bool)
	workspaceOutputs := make(map[string]map[string]interface{})
	runningWorkflows := make(map[string]string) // name -> WorkflowID
	rootFutures := make(map[string]workflow.ChildWorkflowFuture)

	finishedChan := workflow.GetSignalChannel(ctx, SignalWorkspaceFinished)

	// Start root workspaces (those with no dependencies)
	for _, ws := range config.Workspaces {
		if len(ws.DependsOn) == 0 {
			info := workflow.GetInfo(ctx)
			childID := fmt.Sprintf("iac-%s-%s", info.WorkflowExecution.RunID, ws.Name)

			childOptions := workflow.ChildWorkflowOptions{
				WorkflowID: childID,
			}
			if ws.TaskQueue != "" {
				childOptions.TaskQueue = ws.TaskQueue
			}

			ctxChild := workflow.WithChildOptions(ctx, childOptions)
			future := workflow.ExecuteChildWorkflow(ctxChild, TerraformWorkflow, ws)
			rootFutures[ws.Name] = future
			runningWorkflows[ws.Name] = childID
		}
	}

	// Orchestration loop: wait for workspace completions and start ready children
	for len(completedWorkspaces) < len(config.Workspaces) {
		selector := workflow.NewSelector(ctx)
		selector.AddReceive(finishedChan, func(c workflow.ReceiveChannel, more bool) {
			var signal WorkspaceFinishedSignal
			c.Receive(ctx, &signal)

			completedWorkspaces[signal.Name] = true
			workspaceOutputs[signal.Name] = signal.Outputs
			workflow.GetLogger(ctx).Info("Workspace completed", "workspace", signal.Name)

			// Trigger any workspaces that are now ready
			for _, ws := range config.Workspaces {
				if completedWorkspaces[ws.Name] || isRunning(ws.Name, runningWorkflows) {
					continue
				}

				if allDependenciesMet(ws, completedWorkspaces) {
					startWorkspace(ctx, ws, depths, workspaceOutputs, runningWorkflows, rootFutures)
				}
			}
		})

		selector.Select(ctx)
	}

	// Signal shutdown to all hosting workflows
	for _, id := range runningWorkflows {
		if err := workflow.SignalExternalWorkflow(ctx, id, "", SignalShutdown, nil).Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Warn("Failed to send shutdown signal", "workflow_id", id, "error", err)
		}
	}

	// Wait for all root workflows to finish
	var firstErr error
	for name, future := range rootFutures {
		workflow.GetLogger(ctx).Info("Waiting for root workflow to finish", "name", name)
		if err := future.Get(ctx, nil); err != nil {
			workflow.GetLogger(ctx).Error("Root workflow failed", "name", name, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if firstErr != nil {
		return firstErr
	}

	workflow.GetLogger(ctx).Info("Parent workflow completed", "workspaces", len(config.Workspaces))
	return nil
}

func isRunning(name string, running map[string]string) bool {
	_, ok := running[name]
	return ok
}

func allDependenciesMet(ws WorkspaceConfig, completed map[string]bool) bool {
	for _, dep := range ws.DependsOn {
		if !completed[dep] {
			return false
		}
	}
	return true
}

func startWorkspace(
	ctx workflow.Context,
	ws WorkspaceConfig,
	depths map[string]int,
	workspaceOutputs map[string]map[string]interface{},
	runningWorkflows map[string]string,
	rootFutures map[string]workflow.ChildWorkflowFuture,
) {
	// 1. Resolve inputs from dependencies
	if len(ws.Inputs) > 0 {
		if ws.ExtraVars == nil {
			ws.ExtraVars = make(map[string]interface{})
		}
		for _, mapping := range ws.Inputs {
			sourceOuts := workspaceOutputs[mapping.SourceWorkspace]
			if val, ok := sourceOuts[mapping.SourceOutput]; ok {
				// Preserve the original JSON type (string, array, object, etc.)
				ws.ExtraVars[mapping.TargetVar] = val
			}
		}
	}

	// 2. Determine if we should nest or start a new root
	if len(ws.DependsOn) > 0 {
		// Nest under the "deepest" dependency to maintain a logical hierarchy
		hostName := ws.DependsOn[0]
		maxDepth := depths[hostName]
		for _, dep := range ws.DependsOn {
			if depths[dep] > maxDepth {
				maxDepth = depths[dep]
				hostName = dep
			}
		}
		hostID := runningWorkflows[hostName]

		// Signal host to start child
		err := workflow.SignalExternalWorkflow(ctx, hostID, "", SignalStartChild, StartChildSignal{
			Workspace: ws,
		}).Get(ctx, nil)

		if err == nil {
			info := workflow.GetInfo(ctx)
			childID := fmt.Sprintf("iac-%s-%s", info.WorkflowExecution.RunID, ws.Name)
			runningWorkflows[ws.Name] = childID
			return
		}

		workflow.GetLogger(ctx).Warn("Failed to signal host, falling back to root workflow",
			"workspace", ws.Name,
			"host", hostName,
			"error", err,
		)
	}

	// 3. Start as root workflow (either no deps, or signal failed)
	info := workflow.GetInfo(ctx)
	childID := fmt.Sprintf("iac-%s-%s", info.WorkflowExecution.RunID, ws.Name)

	childOptions := workflow.ChildWorkflowOptions{
		WorkflowID: childID,
	}
	if ws.TaskQueue != "" {
		childOptions.TaskQueue = ws.TaskQueue
	}

	ctxChild := workflow.WithChildOptions(ctx, childOptions)
	future := workflow.ExecuteChildWorkflow(ctxChild, TerraformWorkflow, ws)
	rootFutures[ws.Name] = future
	runningWorkflows[ws.Name] = childID
}
