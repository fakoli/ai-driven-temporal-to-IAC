// Package main provides a CLI tool for starting Temporal-based Terraform
// orchestration workflows from a YAML configuration file.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/fakoli/temporal-terraform-orchestrator/utils"
	"github.com/fakoli/temporal-terraform-orchestrator/workflow"
	"go.temporal.io/sdk/client"
)

func main() {
	configPath := flag.String("config", "infra.yaml", "path to infrastructure YAML config")
	taskQueue := flag.String("task-queue", utils.TaskQueue, "Temporal task queue to use")
	workflowID := flag.String("workflow-id", utils.WorkflowID, "Temporal workflow ID")
	flag.Parse()

	cfg, err := workflow.LoadConfigFromFile(*configPath)
	if err != nil {
		log.Fatalf("Unable to load config file %s: %v", *configPath, err)
	}

	if err := workflow.ValidateInfrastructureConfig(cfg); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}
	cfg = workflow.NormalizeInfrastructureConfig(cfg)

	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalln("Unable to create client", err)
	}
	defer c.Close()

	workflowOptions := client.StartWorkflowOptions{
		ID:        *workflowID,
		TaskQueue: *taskQueue,
	}

	we, err := c.ExecuteWorkflow(context.Background(), workflowOptions, workflow.ParentWorkflow, cfg)
	if err != nil {
		log.Fatalln("Unable to execute workflow", err)
	}

	log.Println("Started workflow", "WorkflowID", we.GetID(), "RunID", we.GetRunID())

	var result error
	err = we.Get(context.Background(), &result)
	if err != nil {
		log.Fatalln("Workflow failed", err)
	}

	log.Println("Workflow completed successfully")
}
