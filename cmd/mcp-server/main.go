// Package main implements an MCP (Model Context Protocol) server that enables
// AI agents and automation tools to trigger and monitor Temporal-based
// Terraform orchestration workflows.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/fakoli/temporal-terraform-orchestrator/utils"
	"github.com/fakoli/temporal-terraform-orchestrator/validation"
	"github.com/fakoli/temporal-terraform-orchestrator/workflow"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.temporal.io/sdk/client"
)

func main() {
	// 1. Initialize Temporal Client
	c, err := client.Dial(client.Options{})
	if err != nil {
		log.Fatalf("Unable to create Temporal client: %v", err)
	}
	defer c.Close()

	// 2. Create MCP Server
	s := server.NewMCPServer("terraform-temporal-mcp", "1.0.0")

	// --- Tool: list_workflows ---
	s.AddTool(mcp.NewTool("list_workflows",
		mcp.WithDescription("List available Temporal workflows and configured workspaces from infra.yaml"),
		mcp.WithString("config_path", mcp.Description("Path to YAML config file (defaults to infra.yaml)")),
	), listWorkflowsHandler)

	// --- Tool: execute_workflow ---
	s.AddTool(mcp.NewTool("execute_workflow",
		mcp.WithDescription("Execute a terraform orchestration workflow"),
		mcp.WithString("workflow_name", mcp.Description("Name of the workflow (e.g. ParentWorkflow)"), mcp.Required()),
		mcp.WithString("config_path", mcp.Description("Path to YAML config on server")),
		mcp.WithObject("config", mcp.Description("Inline configuration payload (JSON)")),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return executeWorkflowHandler(ctx, c, request)
	})

	// --- Tool: get_workflow_status ---
	s.AddTool(mcp.NewTool("get_workflow_status",
		mcp.WithDescription("Get the status of a specific workflow execution"),
		mcp.WithString("workflow_id", mcp.Description("The ID of the workflow to check"), mcp.Required()),
	), func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return getWorkflowStatusHandler(ctx, c, request)
	})

	// --- Tool: validate_tfvars ---
	s.AddTool(mcp.NewTool("validate_tfvars",
		mcp.WithDescription("Validate Terraform variables against CEL rules before execution. Returns validation status with detailed error messages and remediation suggestions."),
		mcp.WithString("config_path", mcp.Description("Path to YAML config file"), mcp.Required()),
		mcp.WithString("workspace_name", mcp.Description("Specific workspace to validate (optional, validates all if not specified)")),
		mcp.WithString("rules_path", mcp.Description("Custom rules directory path (optional)")),
		mcp.WithBoolean("fail_on_warning", mcp.Description("Treat warnings as errors (default: false)")),
	), validateTFVarsHandler)

	// Start server on stdio
	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func listWorkflowsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configPath := mcp.ParseString(request, "config_path", "infra.yaml")

	// Try to load the config file
	config, err := workflow.LoadConfigFromFile(configPath)
	if err != nil {
		// If file doesn't exist, return static schema info
		info := map[string]interface{}{
			"error": fmt.Sprintf("Config file not found: %s. Using default schema.", configPath),
			"workflows": []map[string]interface{}{
				{
					"name":        "ParentWorkflow",
					"description": "Orchestrates terraform operations across multiple workspaces with dependencies",
					"input_schema": map[string]interface{}{
						"workspace_root": "string (optional base path)",
						"workspaces": []map[string]interface{}{
							{
								"name":      "string",
								"kind":      "string (default: terraform)",
								"dir":       "string (path to terraform dir)",
								"tfvars":    "string (optional path to tfvars)",
								"dependsOn": "array<string>",
								"taskQueue": "string (optional override)",
							},
						},
					},
				},
			},
		}
		res, err := json.MarshalIndent(info, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
		}
		return mcp.NewToolResultText(string(res)), nil
	}

	// Validate the config
	if err := workflow.ValidateInfrastructureConfig(config); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid config: %v", err)), nil
	}

	// Normalize paths
	config = workflow.NormalizeInfrastructureConfig(config)

	// Build response with actual workspace data
	workspaces := make([]map[string]interface{}, 0, len(config.Workspaces))
	for _, ws := range config.Workspaces {
		wsInfo := map[string]interface{}{
			"name":      ws.Name,
			"kind":      ws.Kind,
			"dir":       ws.Dir,
			"dependsOn": ws.DependsOn,
		}
		if ws.TFVars != "" {
			wsInfo["tfvars"] = ws.TFVars
		}
		if ws.TaskQueue != "" {
			wsInfo["taskQueue"] = ws.TaskQueue
		}
		workspaces = append(workspaces, wsInfo)
	}

	info := map[string]interface{}{
		"config_path":    configPath,
		"workspace_root": config.WorkspaceRoot,
		"workflows": []map[string]interface{}{
			{
				"name":                  "ParentWorkflow",
				"description":           "Orchestrates terraform operations across multiple workspaces with dependencies",
				"configured_workspaces": workspaces,
				"workspace_count":       len(workspaces),
			},
		},
	}

	res, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(res)), nil
}

func executeWorkflowHandler(ctx context.Context, c client.Client, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := mcp.ParseString(request, "workflow_name", "")
	configPath := mcp.ParseString(request, "config_path", "")
	configRaw := mcp.ParseStringMap(request, "config", nil)

	if name != "ParentWorkflow" {
		return mcp.NewToolResultError(fmt.Sprintf("Unsupported workflow: %s", name)), nil
	}

	var config workflow.InfrastructureConfig

	switch {
	case configPath != "":
		var err error
		config, err = workflow.LoadConfigFromFile(configPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to load config: %v", err)), nil
		}
	case configRaw != nil:
		configBytes, _ := json.Marshal(configRaw)
		if err := json.Unmarshal(configBytes, &config); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid config format: %v", err)), nil
		}
	default:
		return mcp.NewToolResultError("Provide config_path or config"), nil
	}

	if err := workflow.ValidateInfrastructureConfig(config); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid config: %v", err)), nil
	}
	config = workflow.NormalizeInfrastructureConfig(config)

	workflowOptions := client.StartWorkflowOptions{
		ID:        fmt.Sprintf("%s-%d", utils.WorkflowID, os.Getpid()),
		TaskQueue: utils.TaskQueue,
	}

	we, err := c.ExecuteWorkflow(ctx, workflowOptions, workflow.ParentWorkflow, config)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to start workflow: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Workflow started successfully.\nWorkflowID: %s\nRunID: %s", we.GetID(), we.GetRunID())), nil
}

func getWorkflowStatusHandler(ctx context.Context, c client.Client, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workflowID := mcp.ParseString(request, "workflow_id", "")

	resp, err := c.DescribeWorkflowExecution(ctx, workflowID, "")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Could not find workflow with ID %s: %v", workflowID, err)), nil
	}

	info := resp.GetWorkflowExecutionInfo()
	status := info.GetStatus().String()
	startTime := info.GetStartTime().AsTime().Format("2006-01-02 15:04:05")

	resultText := fmt.Sprintf("Workflow: %s\nStatus: %s\nStarted At: %s", workflowID, status, startTime)
	if info.GetCloseTime() != nil {
		resultText += fmt.Sprintf("\nFinished At: %s", info.GetCloseTime().AsTime().Format("2006-01-02 15:04:05"))
	}

	return mcp.NewToolResultText(resultText), nil
}

func validateTFVarsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	configPath := mcp.ParseString(request, "config_path", "infra.yaml")
	workspaceName := mcp.ParseString(request, "workspace_name", "")
	rulesPath := mcp.ParseString(request, "rules_path", validation.DefaultRulesPath)

	// Parse fail_on_warning boolean from arguments
	failOnWarning := false
	if args, ok := request.Params.Arguments.(map[string]interface{}); ok {
		if val, exists := args["fail_on_warning"]; exists {
			if boolVal, ok := val.(bool); ok {
				failOnWarning = boolVal
			}
		}
	}

	// Load config
	config, err := workflow.LoadConfigFromFile(configPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to load config: %v", err)), nil
	}

	// Validate structure first
	if err := workflow.ValidateInfrastructureConfig(config); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Invalid config structure: %v", err)), nil
	}

	// Normalize config
	config = workflow.NormalizeInfrastructureConfig(config)

	// Initialize validation service
	svc, err := validation.NewService(rulesPath)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to initialize validation service: %v", err)), nil
	}

	// Prepare results
	response := validation.ValidationResponse{
		Status:     "complete",
		Workspaces: make(map[string]validation.ValidationResult),
		Summary: validation.ValidationSummary{
			TotalWorkspaces: 0,
		},
	}

	// Validate workspaces
	for _, ws := range config.Workspaces {
		// Skip if specific workspace requested and this isn't it
		if workspaceName != "" && ws.Name != workspaceName {
			continue
		}

		response.Summary.TotalWorkspaces++

		// Load tfvars for this workspace
		tfvars := make(map[string]interface{})
		if ws.TFVars != "" {
			data, err := os.ReadFile(ws.TFVars)
			if err != nil {
				// Create error result for this workspace
				result := validation.NewValidationResult()
				result.AddError(validation.ValidationIssue{
					Message:  fmt.Sprintf("Failed to read tfvars: %v", err),
					Severity: validation.SeverityError,
				})
				response.Workspaces[ws.Name] = result
				response.Summary.FailedWorkspaces++
				response.Summary.TotalErrors++
				response.Status = "incomplete"
				continue
			}
			if err := json.Unmarshal(data, &tfvars); err != nil {
				result := validation.NewValidationResult()
				result.AddError(validation.ValidationIssue{
					Message:  fmt.Sprintf("Failed to parse tfvars JSON: %v", err),
					Severity: validation.SeverityError,
				})
				response.Workspaces[ws.Name] = result
				response.Summary.FailedWorkspaces++
				response.Summary.TotalErrors++
				response.Status = "incomplete"
				continue
			}
		}

		// Merge extra vars
		for k, v := range ws.ExtraVars {
			tfvars[k] = v
		}

		// Create workspace context
		wsCtx := validation.WorkspaceContext{
			Name: ws.Name,
			Kind: ws.Kind,
			Dir:  ws.Dir,
		}
		if wsCtx.Kind == "" {
			wsCtx.Kind = "terraform"
		}

		// Validate
		result := svc.ValidateTFVars(tfvars, wsCtx)
		response.Workspaces[ws.Name] = result

		// Update summary
		if result.Valid {
			if failOnWarning && len(result.Warnings) > 0 {
				response.Summary.FailedWorkspaces++
				response.Status = "incomplete"
			} else {
				response.Summary.ValidWorkspaces++
			}
		} else {
			response.Summary.FailedWorkspaces++
			response.Status = "incomplete"
		}
		response.Summary.TotalErrors += len(result.Errors)
		response.Summary.TotalWarnings += len(result.Warnings)
	}

	// Format response
	resJSON, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}

	// Return as error if validation failed
	if response.Status == "incomplete" {
		return mcp.NewToolResultError(string(resJSON)), nil
	}

	return mcp.NewToolResultText(string(resJSON)), nil
}
