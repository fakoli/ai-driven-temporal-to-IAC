# Temporal Terraform Orchestrator

A Temporal-based workflow orchestration system for managing multi-workspace Terraform deployments with dependency resolution, variable passing between workspaces, and MCP server integration for AI-driven automation.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [CLI Starter](#cli-starter)
- [MCP Server](#mcp-server)
- [Configuration Reference (`infra.yaml`)](#configuration-reference-infrayaml)
- [Testing](#testing)
- [Repository Layout](#repository-layout)
- [Troubleshooting](#troubleshooting)

## Overview

This project provides a robust infrastructure-as-code orchestration layer using [Temporal](https://temporal.io/) workflows to manage Terraform operations across multiple workspaces with:

- **Dependency management**: Define workspace dependencies and the system executes them in the correct order
- **Output propagation**: Pass Terraform outputs from one workspace as inputs to dependent workspaces
- **Parallel execution**: Independent workspaces run concurrently for faster deployments
- **Durability**: Temporal provides automatic retries, state persistence, and failure recovery
- **AI integration**: MCP server enables AI agents to trigger and monitor infrastructure deployments

```
ParentWorkflow (Orchestrator)
  │
  ├─> TerraformWorkflow (vpc)          ← runs first (no dependencies)
  │
  ├─> TerraformWorkflow (vpc-2)        ← runs in parallel with vpc
  │
  ├─> TerraformWorkflow (subnets)      ← waits for vpc, receives vpc_id output
  │
  └─> TerraformWorkflow (eks)          ← waits for vpc + subnets, receives both outputs
```

## Architecture

### Workflow Components

**ParentWorkflow**: The orchestrator that:

1. Validates and normalizes the configuration
2. Builds a dependency DAG (Directed Acyclic Graph)
3. Starts root workspaces (those with no dependencies) immediately
4. Listens for completion signals and starts dependent workspaces when ready
5. Propagates outputs between workspaces based on input mappings

**TerraformWorkflow**: Executes Terraform operations for a single workspace:

1. `terraform init` - Initialize the workspace
2. `terraform plan` - Create execution plan (with `-detailed-exitcode` to detect changes)
3. `terraform show -json` - Validate the plan output
4. `terraform apply` - Apply changes (skipped if no changes detected)
5. `terraform output -json` - Capture outputs for downstream workspaces
6. Signals completion back to ParentWorkflow
7. Enters "hosting mode" to spawn child workflows for nested dependencies

### Hosting Architecture

Child workflows are spawned as nested children of their "host" workflow (the deepest dependency). This creates a natural hierarchy where:

- Root workspaces are direct children of ParentWorkflow
- Dependent workspaces become children of their host workflow
- All workflows signal completion back to the ParentWorkflow orchestrator

## Prerequisites

- **Go 1.23+**
- **Terraform CLI >= 1.5** - Available on PATH
- **Temporal Server** - Running at default address `localhost:7233` (or set `TEMPORAL_ADDRESS`)
- **AWS Credentials** - If using real AWS resources (the examples use mock outputs)

## Quick Start

1. **Install dependencies**:

   ```bash
   go mod tidy
   ```

2. **Start Temporal** (if not already running):

   ```bash
   temporal server start-dev
   ```

3. **Start the worker**:

   ```bash
   go run ./cmd/worker
   ```

4. **Execute the workflow**:

   ```bash
   go run ./cmd/starter -config infra.yaml
   ```

5. **Monitor execution** in the Temporal Web UI at `http://localhost:8233`

## CLI Starter

The CLI starter (`cmd/starter`) initiates workflow execution from a YAML configuration file.

### Usage

```bash
go run ./cmd/starter [flags]
```

### Flags

| Flag           | Default                     | Description                                   |
| -------------- | --------------------------- | --------------------------------------------- |
| `-config`      | `infra.yaml`                | Path to the infrastructure configuration file |
| `-task-queue`  | `terraform-task-queue`      | Temporal task queue name                      |
| `-workflow-id` | `terraform-parent-workflow` | Custom workflow ID for tracking               |

### Examples

```bash
# Run with default settings
go run ./cmd/starter

# Specify a custom config file
go run ./cmd/starter -config ./environments/production.yaml

# Use a custom workflow ID for tracking
go run ./cmd/starter -config infra.yaml -workflow-id "deploy-prod-2024-01-15"
```

### Behavior

1. Reads and parses the YAML configuration file
2. Validates the configuration (checks for cycles, missing dependencies, etc.)
3. Normalizes paths relative to `workspace_root`
4. Starts the ParentWorkflow via Temporal
5. Waits for workflow completion and reports success/failure

## MCP Server

The MCP (Model Context Protocol) server enables AI agents and automation tools to interact with the orchestration system.

### Starting the Server

```bash
go run ./cmd/mcp-server
```

The server runs on stdio and communicates via JSON-RPC, following the MCP specification.

### Available Tools

#### `list_workflows`

Lists available workflows and configured workspaces from the configuration file.

**Parameters:**
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `config_path` | string | No | `infra.yaml` | Path to YAML config file |

**Response example:**

```json
{
  "config_path": "infra.yaml",
  "workspace_root": ".",
  "workflows": [
    {
      "name": "ParentWorkflow",
      "description": "Orchestrates terraform operations across multiple workspaces with dependencies",
      "configured_workspaces": [
        {
          "name": "vpc",
          "kind": "terraform",
          "dir": "/abs/path/vpc",
          "dependsOn": []
        },
        {
          "name": "subnets",
          "kind": "terraform",
          "dir": "/abs/path/subnets",
          "dependsOn": ["vpc"]
        }
      ],
      "workspace_count": 2
    }
  ]
}
```

#### `execute_workflow`

Starts a Terraform orchestration workflow.

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workflow_name` | string | Yes | Must be `ParentWorkflow` |
| `config_path` | string | No* | Path to YAML config file |
| `config` | object | No* | Inline configuration payload (JSON) |

\*Either `config_path` or `config` must be provided.

**Response example:**

```
Workflow started successfully.
WorkflowID: terraform-parent-workflow-12345
RunID: abc123-def456-ghi789
```

#### `get_workflow_status`

Gets the status of a running or completed workflow.

**Parameters:**
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `workflow_id` | string | Yes | The workflow ID to check |

**Response example:**

```
Workflow: terraform-parent-workflow-12345
Status: WORKFLOW_EXECUTION_STATUS_COMPLETED
Started At: 2024-01-15 10:30:00
Finished At: 2024-01-15 10:35:42
```

### Integration with AI Agents

The MCP server is designed for integration with AI coding assistants (like Cursor, Claude, etc.). Add it to your MCP configuration:

```json
{
  "mcpServers": {
    "temporal-terraform": {
      "command": "/path/to/mcp-server",
      "args": []
    }
  }
}
```

## Configuration Reference (`infra.yaml`)

The configuration file defines your infrastructure workspaces and their relationships.

### Complete Schema

```yaml
# Base path for resolving relative directories (optional)
workspace_root: '.'

# List of workspaces to orchestrate
workspaces:
  - name: string # Required: Unique workspace identifier
    kind: string # Optional: "terraform" (default and only supported value)
    dir: string # Required: Path to Terraform directory
    tfvars: string # Optional: Path to .tfvars file
    dependsOn: [string] # Optional: List of workspace names this depends on
    inputs: [InputMapping] # Optional: Variable mappings from dependencies
    operations: [string] # Optional: Operations to run (default: [init, validate, plan, apply])
    taskQueue: string # Optional: Override the Temporal task queue
```

### Input Mapping Schema

```yaml
inputs:
  - sourceWorkspace: string # Name of the dependency workspace
    sourceOutput: string # Name of the Terraform output to read
    targetVar: string # Name of the Terraform variable to set
```

### Complete Example

```yaml
workspace_root: .

workspaces:
  # VPC workspace - no dependencies, runs first
  - name: vpc
    kind: terraform
    dir: terraform/examples/vpc
    tfvars: terraform/examples/vpc/vpc.tfvars
    dependsOn: []
    taskQueue: terraform-task-queue

  # Second VPC - independent, runs in parallel with first
  - name: vpc-2
    kind: terraform
    dir: terraform/examples/vpc-2
    tfvars: terraform/examples/vpc-2/vpc.tfvars
    dependsOn: []
    taskQueue: terraform-task-queue

  # Subnets - depends on VPC, receives vpc_id output
  - name: subnets
    kind: terraform
    dir: terraform/examples/subnets
    tfvars: terraform/examples/subnets/subnets.tfvars
    dependsOn:
      - vpc
    inputs:
      - sourceWorkspace: vpc
        sourceOutput: vpc_id
        targetVar: vpc_id
    taskQueue: terraform-task-queue

  # EKS - depends on both VPC and subnets, receives outputs from both
  - name: eks
    kind: terraform
    dir: terraform/examples/eks
    tfvars: terraform/examples/eks/eks.tfvars
    dependsOn:
      - vpc
      - subnets
    inputs:
      - sourceWorkspace: vpc
        sourceOutput: vpc_id
        targetVar: vpc_id
      - sourceWorkspace: subnets
        sourceOutput: subnet_ids
        targetVar: subnet_ids
```

### Key Concepts

#### Workspace Dependencies (`dependsOn`)

- Define execution order constraints
- Multiple dependencies create AND conditions (all must complete)
- Independent workspaces (empty or no `dependsOn`) run in parallel
- Cycles are detected and rejected during validation

#### Output to Input Propagation (`inputs`)

The `inputs` array maps Terraform outputs from dependency workspaces to variables in the current workspace:

```yaml
# In the subnets workspace config
inputs:
  - sourceWorkspace: vpc # Get output from 'vpc' workspace
    sourceOutput: vpc_id # Read the 'vpc_id' output
    targetVar: vpc_id # Pass as -var vpc_id=<value>
```

This allows you to:

- Chain infrastructure components together
- Pass resource IDs between workspaces
- Build complex dependency graphs

#### Transitive Dependencies

Input mappings support transitive dependencies. For example, if `C` depends on `B`, and `B` depends on `A`, then `C` can map outputs from both `B` AND `A`:

```yaml
workspaces:
  - name: a
    dir: ./a

  - name: b
    dir: ./b
    dependsOn: [a]

  - name: c
    dir: ./c
    dependsOn: [b]
    inputs:
      - sourceWorkspace: a # Valid: 'a' is a transitive dependency
        sourceOutput: some_output
        targetVar: from_a
      - sourceWorkspace: b # Valid: 'b' is a direct dependency
        sourceOutput: other_output
        targetVar: from_b
```

#### Operations Control

The `operations` field allows fine-grained control over which Terraform operations to run for each workspace:

```yaml
operations: [init, validate, plan, apply]  # Full apply mode (default)
operations: [init, validate, plan]         # Plan-only mode (no apply)
```

**Valid operations:**

- `init` - Initialize the Terraform workspace (required)
- `validate` - Validate Terraform configuration (required)
- `plan` - Generate execution plan
- `apply` - Apply changes to infrastructure

**Requirements:**

- `init` and `validate` are always required
- Operations must be specified in order: `init` → `validate` → `plan` → `apply`
- `apply` requires `plan` to be present

**Use cases:**

- **Plan-only mode**: Set `operations: [init, validate, plan]` for review/approval workflows
- **Full apply mode**: Set `operations: [init, validate, plan, apply]` for automatic deployments (default)

#### Path Resolution

- `workspace_root`: Base path for resolving relative paths
- Relative paths in `dir` and `tfvars` are joined with `workspace_root`
- Absolute paths are used as-is
- The current working directory is used if `workspace_root` is empty

## Testing

Run all tests:

```bash
go test ./...
```

### Test Coverage

- **Config validation**: Cycle detection, duplicate names, missing dependencies, input mapping validation
- **Parent workflow**: Execution order, dependency waiting, signal handling
- **Activities**: Uses a shim Terraform binary that simulates CLI behavior

The tests use a fake Terraform binary that:

- Returns exit code 2 for `plan` (simulating changes)
- Creates plan files on disk
- Returns mock JSON for `output` and `show` commands

## Repository Layout

```
.
├── activities/                 # Terraform CLI wrapper activities
│   ├── terraform_activities.go # Init, Plan, Validate, Apply, Output
│   └── terraform_activities_test.go
├── cmd/
│   ├── mcp-server/            # MCP server for AI integration
│   ├── starter/               # CLI to start workflows
│   └── worker/                # Temporal worker process
├── terraform/examples/        # Sample Terraform workspaces
│   ├── vpc/
│   ├── vpc-2/
│   ├── subnets/
│   └── eks/
├── utils/                     # Shared constants
├── workflow/                  # Temporal workflow definitions
│   ├── config.go              # Configuration types and validation
│   ├── parent_workflow.go     # Orchestrator workflow
│   └── terraform_workflow.go  # Per-workspace workflow
├── go.mod
├── go.sum
├── infra.yaml                 # Example configuration
└── README.md
```

## Troubleshooting

### Worker Connection Issues

```
Unable to create client: ...
```

**Solution**: Ensure Temporal server is running and accessible. Check `TEMPORAL_ADDRESS` if using a non-default location.

### Workflow Fails Immediately

```
Invalid config: ...
```

**Solution**: Validate your `infra.yaml`:

- Check for duplicate workspace names
- Ensure all `dependsOn` references exist
- Verify there are no circular dependencies
- Confirm `inputs` reference valid dependencies

### Terraform Errors

```
terraform init failed: ...
```

**Solutions**:

- Ensure `terraform` is on PATH
- Verify the workspace `dir` path exists
- Check for valid Terraform configuration in the directory
- For AWS resources, ensure credentials are configured

### Plan File Not Found

```
plan file not found for apply: ...
```

**Solution**: This typically indicates `terraform plan` failed silently. Check:

- Directory permissions
- Terraform initialization state
- Provider configurations

### MCP Server Not Responding

**Solution**: The MCP server runs on stdio. Ensure your client is configured to communicate via stdin/stdout, not HTTP.
