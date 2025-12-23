package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstants(t *testing.T) {
	// Test that constants are defined with expected values
	assert.Equal(t, "terraform-task-queue", TaskQueue)
	assert.Equal(t, "terraform-parent-workflow", WorkflowID)
}

func TestTaskQueueNotEmpty(t *testing.T) {
	assert.NotEmpty(t, TaskQueue, "TaskQueue should not be empty")
}

func TestWorkflowIDNotEmpty(t *testing.T) {
	assert.NotEmpty(t, WorkflowID, "WorkflowID should not be empty")
}
