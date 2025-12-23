// Package utils provides shared constants and utility functions
// used across the Temporal Terraform orchestrator.
package utils

const (
	// TaskQueue is the default Temporal task queue name for workflow and activity workers.
	TaskQueue = "terraform-task-queue"

	// WorkflowID is the default identifier for the parent orchestrator workflow.
	WorkflowID = "terraform-parent-workflow"
)
