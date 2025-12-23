// Package main implements the Temporal worker process that executes
// Terraform orchestration workflows and activities.
package main

import (
	"log"

	"github.com/fakoli/temporal-terraform-orchestrator/activities"
	orchestrator "github.com/fakoli/temporal-terraform-orchestrator/workflow"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalln("Unable to create client", err)
	}
	defer c.Close()

	w := worker.New(c, "terraform-task-queue", worker.Options{})

	w.RegisterWorkflow(orchestrator.ParentWorkflow)
	w.RegisterWorkflow(orchestrator.TerraformWorkflow)

	var a *activities.TerraformActivities
	w.RegisterActivity(a)

	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalln("Unable to start worker", err)
	}
}
