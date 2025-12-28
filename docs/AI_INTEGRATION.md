# AI Integration Guide

This document provides comprehensive guidance on integrating AI agents with the Temporal Terraform Orchestrator using the Model Context Protocol (MCP).

## Table of Contents

1. [Overview](#overview)
2. [Model Context Protocol (MCP)](#model-context-protocol-mcp)
3. [MCP Server Implementation](#mcp-server-implementation)
4. [Available Tools](#available-tools)
5. [Integration Patterns](#integration-patterns)
6. [Configuration Examples](#configuration-examples)
7. [Security Considerations](#security-considerations)
8. [Advanced Use Cases](#advanced-use-cases)

---

## Overview

The Temporal Terraform Orchestrator includes an MCP server that enables AI agents to autonomously manage infrastructure. This creates a bridge between natural language interfaces and declarative infrastructure management.

### Why AI Integration?

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    TRADITIONAL vs AI-DRIVEN OPERATIONS                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Traditional Flow:                                                           │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐             │
│  │  Human   │───▶│  Write   │───▶│  Review  │───▶│  Execute │             │
│  │  Intent  │    │  Config  │    │  Config  │    │  Deploy  │             │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘             │
│       │               │               │               │                     │
│       │ "Deploy new   │ Write YAML    │ Check for     │ Run commands       │
│       │  staging      │ manually      │ errors        │ manually           │
│       │  environment" │               │               │                     │
│                                                                              │
│  AI-Driven Flow:                                                             │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐    ┌──────────┐             │
│  │  Human   │───▶│    AI    │───▶│   MCP    │───▶│ Temporal │             │
│  │  Intent  │    │  Agent   │    │  Server  │    │ Workflow │             │
│  └──────────┘    └──────────┘    └──────────┘    └──────────┘             │
│       │               │               │               │                     │
│       │ "Deploy new   │ Interprets    │ Structured    │ Automated          │
│       │  staging      │ and plans     │ API calls     │ execution          │
│       │  environment" │               │               │                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Benefits

| Benefit | Description |
|---------|-------------|
| **Natural Language Interface** | Operators describe intent in plain English |
| **Automated Planning** | AI determines correct sequence of operations |
| **Context Awareness** | AI understands relationships between resources |
| **Proactive Monitoring** | AI can check status and respond to issues |
| **Audit Trail** | All AI actions are logged through Temporal |

---

## Model Context Protocol (MCP)

### What is MCP?

The Model Context Protocol is a standardized way for AI agents to interact with external tools and services. It provides:

1. **Tool Discovery**: AI agents can discover available capabilities
2. **Structured Invocation**: Tools are called with typed parameters
3. **Consistent Responses**: Results are returned in a standard format

### Protocol Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MCP COMMUNICATION FLOW                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   AI Agent (Claude, GPT, etc.)                                              │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                                                                      │   │
│   │  1. User: "Deploy the staging environment"                          │   │
│   │                                                                      │   │
│   │  2. AI reasons about available tools                                │   │
│   │     - list_workflows: discover what's configured                    │   │
│   │     - execute_workflow: trigger deployment                          │   │
│   │     - get_workflow_status: monitor progress                         │   │
│   │                                                                      │   │
│   │  3. AI decides to call list_workflows first                         │   │
│   │                                                                      │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│                          │ JSON-RPC over stdio                              │
│                          ▼                                                   │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  MCP Request:                                                        │   │
│   │  {                                                                   │   │
│   │    "jsonrpc": "2.0",                                                │   │
│   │    "method": "tools/call",                                          │   │
│   │    "params": {                                                       │   │
│   │      "name": "list_workflows",                                       │   │
│   │      "arguments": {"config_path": "staging/infra.yaml"}             │   │
│   │    },                                                                │   │
│   │    "id": 1                                                          │   │
│   │  }                                                                   │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│                          ▼                                                   │
│   MCP Server (cmd/mcp-server)                                               │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                                                                      │   │
│   │  1. Parse request                                                   │   │
│   │  2. Route to handler (listWorkflowsHandler)                         │   │
│   │  3. Load and validate configuration                                 │   │
│   │  4. Format response                                                  │   │
│   │                                                                      │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│                          ▼                                                   │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  MCP Response:                                                       │   │
│   │  {                                                                   │   │
│   │    "jsonrpc": "2.0",                                                │   │
│   │    "result": {                                                       │   │
│   │      "content": [{                                                   │   │
│   │        "type": "text",                                               │   │
│   │        "text": "{\"workflows\": [...], \"workspace_count\": 4}"     │   │
│   │      }]                                                              │   │
│   │    },                                                                │   │
│   │    "id": 1                                                          │   │
│   │  }                                                                   │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│                          ▼                                                   │
│   AI Agent continues reasoning with new information...                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## MCP Server Implementation

### Server Architecture

```go
// cmd/mcp-server/main.go

func main() {
    // 1. Initialize Temporal Client
    c, err := client.Dial(client.Options{})

    // 2. Create MCP Server
    s := server.NewMCPServer("terraform-temporal-mcp", "1.0.0")

    // 3. Register Tools
    s.AddTool(mcp.NewTool("list_workflows", ...), listWorkflowsHandler)
    s.AddTool(mcp.NewTool("execute_workflow", ...), executeWorkflowHandler)
    s.AddTool(mcp.NewTool("get_workflow_status", ...), getWorkflowStatusHandler)

    // 4. Start Server on stdio
    server.ServeStdio(s)
}
```

### Component Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          MCP SERVER COMPONENTS                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │                         MCP Server                                   │   │
│   │   ┌─────────────────────────────────────────────────────────────┐   │   │
│   │   │                    Tool Registry                             │   │   │
│   │   │  ┌──────────────┐ ┌──────────────┐ ┌──────────────────────┐│   │   │
│   │   │  │list_workflows│ │execute_      │ │get_workflow_status   ││   │   │
│   │   │  │              │ │workflow      │ │                      ││   │   │
│   │   │  │ Parameters:  │ │              │ │ Parameters:          ││   │   │
│   │   │  │ - config_path│ │ Parameters:  │ │ - workflow_id (req)  ││   │   │
│   │   │  │   (optional) │ │ - workflow_  │ │                      ││   │   │
│   │   │  │              │ │   name (req) │ │ Returns:             ││   │   │
│   │   │  │ Returns:     │ │ - config_path│ │ - Status             ││   │   │
│   │   │  │ - Workspace  │ │ - config     │ │ - Start time         ││   │   │
│   │   │  │   list       │ │   (inline)   │ │ - End time           ││   │   │
│   │   │  │ - Dependencies│ │             │ │                      ││   │   │
│   │   │  └──────────────┘ └──────────────┘ └──────────────────────┘│   │   │
│   │   └─────────────────────────────────────────────────────────────┘   │   │
│   │                              │                                       │   │
│   │                              ▼                                       │   │
│   │   ┌─────────────────────────────────────────────────────────────┐   │   │
│   │   │                   Handler Functions                          │   │   │
│   │   │                                                              │   │   │
│   │   │  listWorkflowsHandler()                                      │   │   │
│   │   │    └── workflow.LoadConfigFromFile()                        │   │   │
│   │   │    └── workflow.ValidateInfrastructureConfig()              │   │   │
│   │   │    └── Format workspace list as JSON                        │   │   │
│   │   │                                                              │   │   │
│   │   │  executeWorkflowHandler()                                    │   │   │
│   │   │    └── Load config (file or inline)                         │   │   │
│   │   │    └── Validate and normalize                               │   │   │
│   │   │    └── c.ExecuteWorkflow(ParentWorkflow, config)            │   │   │
│   │   │    └── Return WorkflowID and RunID                          │   │   │
│   │   │                                                              │   │   │
│   │   │  getWorkflowStatusHandler()                                  │   │   │
│   │   │    └── c.DescribeWorkflowExecution(workflowID)              │   │   │
│   │   │    └── Format status, timestamps                            │   │   │
│   │   └─────────────────────────────────────────────────────────────┘   │   │
│   │                              │                                       │   │
│   │                              ▼                                       │   │
│   │   ┌─────────────────────────────────────────────────────────────┐   │   │
│   │   │                   Temporal Client                            │   │   │
│   │   │                                                              │   │   │
│   │   │  - Connected to Temporal Server                              │   │   │
│   │   │  - ExecuteWorkflow: Start new workflows                      │   │   │
│   │   │  - DescribeWorkflowExecution: Get status                     │   │   │
│   │   └─────────────────────────────────────────────────────────────┘   │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│   Communication: stdio (stdin/stdout)                                        │
│   Protocol: JSON-RPC 2.0                                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Available Tools

### Tool: list_workflows

**Purpose**: Discover configured workspaces and their dependencies.

**Parameters**:
| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| config_path | string | No | "infra.yaml" | Path to configuration file |

**Example Request**:
```json
{
  "name": "list_workflows",
  "arguments": {
    "config_path": "environments/production/infra.yaml"
  }
}
```

**Example Response**:
```json
{
  "config_path": "environments/production/infra.yaml",
  "workspace_root": ".",
  "workflows": [
    {
      "name": "ParentWorkflow",
      "description": "Orchestrates terraform operations across multiple workspaces with dependencies",
      "configured_workspaces": [
        {
          "name": "vpc",
          "kind": "terraform",
          "dir": "/absolute/path/to/vpc",
          "dependsOn": [],
          "operations": ["init", "validate", "plan", "apply"]
        },
        {
          "name": "subnets",
          "kind": "terraform",
          "dir": "/absolute/path/to/subnets",
          "dependsOn": ["vpc"],
          "operations": ["init", "validate", "plan", "apply"]
        },
        {
          "name": "eks",
          "kind": "terraform",
          "dir": "/absolute/path/to/eks",
          "dependsOn": ["vpc", "subnets"],
          "operations": ["init", "validate", "plan"]
        }
      ],
      "workspace_count": 3
    }
  ]
}
```

### Tool: execute_workflow

**Purpose**: Trigger infrastructure deployment.

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| workflow_name | string | Yes | Must be "ParentWorkflow" |
| config_path | string | No* | Path to YAML/JSON config file |
| config | object | No* | Inline configuration (JSON) |

*Either config_path or config must be provided.

**Example Request (File-based)**:
```json
{
  "name": "execute_workflow",
  "arguments": {
    "workflow_name": "ParentWorkflow",
    "config_path": "staging/infra.yaml"
  }
}
```

**Example Request (Inline Config)**:
```json
{
  "name": "execute_workflow",
  "arguments": {
    "workflow_name": "ParentWorkflow",
    "config": {
      "workspace_root": "/opt/infrastructure",
      "workspaces": [
        {
          "name": "database",
          "kind": "terraform",
          "dir": "terraform/database",
          "operations": ["init", "validate", "plan", "apply"]
        }
      ]
    }
  }
}
```

**Example Response**:
```
Workflow started successfully.
WorkflowID: terraform-parent-workflow-12345
RunID: a1b2c3d4-e5f6-7890-abcd-ef1234567890
```

### Tool: get_workflow_status

**Purpose**: Check the status of a running or completed workflow.

**Parameters**:
| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| workflow_id | string | Yes | The workflow ID to check |

**Example Request**:
```json
{
  "name": "get_workflow_status",
  "arguments": {
    "workflow_id": "terraform-parent-workflow-12345"
  }
}
```

**Example Response (Running)**:
```
Workflow: terraform-parent-workflow-12345
Status: WORKFLOW_EXECUTION_STATUS_RUNNING
Started At: 2024-01-15 10:30:00
```

**Example Response (Completed)**:
```
Workflow: terraform-parent-workflow-12345
Status: WORKFLOW_EXECUTION_STATUS_COMPLETED
Started At: 2024-01-15 10:30:00
Finished At: 2024-01-15 10:45:23
```

**Example Response (Failed)**:
```
Workflow: terraform-parent-workflow-12345
Status: WORKFLOW_EXECUTION_STATUS_FAILED
Started At: 2024-01-15 10:30:00
Finished At: 2024-01-15 10:32:15
```

---

## Integration Patterns

### Pattern 1: Conversational Deployment

AI interprets natural language and executes appropriate operations.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CONVERSATIONAL DEPLOYMENT FLOW                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  User: "Deploy the production VPC and wait for it to complete"              │
│                                                                              │
│  AI Agent Processing:                                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                                                                      │   │
│  │  Step 1: Understand intent                                          │   │
│  │  - Action: Deploy                                                   │   │
│  │  - Target: production VPC                                           │   │
│  │  - Requirement: Wait for completion                                 │   │
│  │                                                                      │   │
│  │  Step 2: Discover configuration                                      │   │
│  │  → Call: list_workflows(config_path="production/infra.yaml")        │   │
│  │  ← Response: {workspaces: [{name: "vpc", ...}]}                     │   │
│  │                                                                      │   │
│  │  Step 3: Execute deployment                                          │   │
│  │  → Call: execute_workflow(                                           │   │
│  │           workflow_name="ParentWorkflow",                           │   │
│  │           config_path="production/infra.yaml"                       │   │
│  │         )                                                            │   │
│  │  ← Response: WorkflowID: tf-parent-98765                            │   │
│  │                                                                      │   │
│  │  Step 4: Monitor until complete                                      │   │
│  │  → Call: get_workflow_status(workflow_id="tf-parent-98765")         │   │
│  │  ← Response: Status: RUNNING                                        │   │
│  │  ... (poll periodically)                                             │   │
│  │  → Call: get_workflow_status(workflow_id="tf-parent-98765")         │   │
│  │  ← Response: Status: COMPLETED                                      │   │
│  │                                                                      │   │
│  │  Step 5: Report to user                                              │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  AI: "The production VPC deployment has completed successfully.              │
│       Workflow ID: tf-parent-98765                                           │
│       Started: 10:30:00                                                      │
│       Finished: 10:45:23                                                     │
│       Duration: 15 minutes 23 seconds"                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Pattern 2: Plan-Review-Apply Workflow

AI generates plans for human review before applying.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PLAN-REVIEW-APPLY WORKFLOW                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Phase 1: Generate Plan                                                      │
│  ───────────────────────                                                     │
│                                                                              │
│  User: "Show me what would change if we deployed to production"             │
│                                                                              │
│  AI executes workflow with plan-only configuration:                          │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  {                                                                   │   │
│  │    "workspaces": [                                                   │   │
│  │      {                                                               │   │
│  │        "name": "vpc",                                                │   │
│  │        "operations": ["init", "validate", "plan"]  ← No apply       │   │
│  │      }                                                               │   │
│  │    ]                                                                 │   │
│  │  }                                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  AI: "Plan generated. Here's what would change:                             │
│       - 1 VPC to be created                                                  │
│       - 3 subnets to be created                                              │
│       - 2 security groups to be created                                      │
│       Would you like me to apply these changes?"                             │
│                                                                              │
│  Phase 2: Human Review                                                       │
│  ────────────────────────                                                    │
│                                                                              │
│  User: "Yes, apply the changes"                                              │
│                                                                              │
│  Phase 3: Apply                                                              │
│  ──────────────────                                                          │
│                                                                              │
│  AI executes with full operations:                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  {                                                                   │   │
│  │    "workspaces": [                                                   │   │
│  │      {                                                               │   │
│  │        "name": "vpc",                                                │   │
│  │        "operations": ["init", "validate", "plan", "apply"]          │   │
│  │      }                                                               │   │
│  │    ]                                                                 │   │
│  │  }                                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Pattern 3: Autonomous Monitoring and Remediation

AI proactively monitors and responds to issues.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    AUTONOMOUS REMEDIATION FLOW                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  External Alert System                                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  Alert: EKS cluster health check failing                            │   │
│  │  Cluster: production-eks                                             │   │
│  │  Time: 2024-01-15 14:30:00                                           │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                          │                                                   │
│                          ▼                                                   │
│  AI Agent Receives Alert                                                     │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                                                                      │   │
│  │  Step 1: Analyze alert                                               │   │
│  │  - Resource: EKS cluster                                            │   │
│  │  - Issue: Health check failure                                      │   │
│  │  - Potential causes: Network, node, configuration                   │   │
│  │                                                                      │   │
│  │  Step 2: Check infrastructure state                                  │   │
│  │  → Call: list_workflows(config_path="production/infra.yaml")        │   │
│  │  ← Response: Shows eks depends on vpc, subnets                      │   │
│  │                                                                      │   │
│  │  Step 3: Determine remediation                                       │   │
│  │  - Decision: Re-run EKS workspace to sync state                     │   │
│  │                                                                      │   │
│  │  Step 4: Execute remediation                                         │   │
│  │  → Call: execute_workflow(                                           │   │
│  │           workflow_name="ParentWorkflow",                           │   │
│  │           config={                                                   │   │
│  │             workspaces: [{                                           │   │
│  │               name: "eks",                                           │   │
│  │               operations: ["init", "validate", "plan", "apply"]     │   │
│  │             }]                                                       │   │
│  │           }                                                          │   │
│  │         )                                                            │   │
│  │                                                                      │   │
│  │  Step 5: Monitor result                                              │   │
│  │  → Call: get_workflow_status(...)                                   │   │
│  │  ← Response: COMPLETED                                              │   │
│  │                                                                      │   │
│  │  Step 6: Verify health restored                                      │   │
│  │  - Check EKS health endpoint                                        │   │
│  │  - Confirm resolution                                                │   │
│  │                                                                      │   │
│  │  Step 7: Log and notify                                              │   │
│  │  - Create incident report                                           │   │
│  │  - Notify operations team                                            │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Configuration Examples

### Claude Desktop Configuration

Add to `~/.config/claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "terraform-orchestrator": {
      "command": "/path/to/mcp-server",
      "args": [],
      "env": {
        "TEMPORAL_ADDRESS": "localhost:7233"
      }
    }
  }
}
```

### Cursor IDE Configuration

Add to Cursor MCP settings:

```json
{
  "mcpServers": {
    "terraform-orchestrator": {
      "command": "/path/to/mcp-server",
      "args": []
    }
  }
}
```

### Custom Integration

For programmatic integration:

```python
import subprocess
import json

class MCPClient:
    def __init__(self, server_path):
        self.process = subprocess.Popen(
            [server_path],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            text=True
        )
        self.request_id = 0

    def call_tool(self, tool_name, arguments):
        self.request_id += 1
        request = {
            "jsonrpc": "2.0",
            "method": "tools/call",
            "params": {
                "name": tool_name,
                "arguments": arguments
            },
            "id": self.request_id
        }

        self.process.stdin.write(json.dumps(request) + "\n")
        self.process.stdin.flush()

        response = json.loads(self.process.stdout.readline())
        return response

# Usage
client = MCPClient("/path/to/mcp-server")

# List workflows
result = client.call_tool("list_workflows", {"config_path": "infra.yaml"})
print(result)

# Execute workflow
result = client.call_tool("execute_workflow", {
    "workflow_name": "ParentWorkflow",
    "config_path": "infra.yaml"
})
print(result)
```

---

## Security Considerations

### Access Control

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        SECURITY LAYERS                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Layer 1: Process-Level Security                                             │
│  ────────────────────────────────                                            │
│  - MCP server runs with user permissions                                    │
│  - Access to files limited by filesystem permissions                        │
│  - Terraform credentials via environment variables                          │
│                                                                              │
│  Layer 2: Temporal Security                                                  │
│  ──────────────────────────                                                  │
│  - mTLS for production deployments                                          │
│  - Namespace isolation                                                       │
│  - Role-based access control                                                 │
│                                                                              │
│  Layer 3: Cloud Provider Security                                            │
│  ─────────────────────────────────                                           │
│  - IAM roles with least privilege                                           │
│  - Resource-level permissions                                                │
│  - Audit logging                                                             │
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                    RECOMMENDED SETUP                                 │   │
│  │                                                                      │   │
│  │  AI Agent                                                            │   │
│  │      │                                                               │   │
│  │      ▼                                                               │   │
│  │  MCP Server (restricted user)                                       │   │
│  │      │                                                               │   │
│  │      ▼                                                               │   │
│  │  Temporal Server (mTLS enabled)                                     │   │
│  │      │                                                               │   │
│  │      ▼                                                               │   │
│  │  Terraform (assumed IAM role)                                       │   │
│  │      │                                                               │   │
│  │      ▼                                                               │   │
│  │  Cloud Resources (tagged, audited)                                  │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Best Practices

1. **Credential Management**
   - Never store credentials in configuration files
   - Use environment variables or credential helpers
   - Rotate credentials regularly

2. **Audit Logging**
   - All MCP calls are logged by Temporal
   - Enable cloud provider audit trails
   - Monitor for unusual patterns

3. **Blast Radius Control**
   - Use separate configurations per environment
   - Limit AI agent permissions appropriately
   - Implement approval workflows for production

4. **Input Validation**
   - MCP server validates all inputs
   - Configuration is validated before execution
   - Malformed requests are rejected

---

## Advanced Use Cases

### Multi-Environment Deployment

```
User: "Deploy the same configuration to staging and production,
       but only plan for production"

AI Agent:
1. Execute staging with full apply:
   → execute_workflow(config_path="staging/infra.yaml")

2. Execute production with plan only:
   → execute_workflow(
       config={
         workspaces: [
           {name: "vpc", operations: ["init", "validate", "plan"]},
           {name: "subnets", operations: ["init", "validate", "plan"]}
         ]
       }
     )

3. Report both results with diff summary
```

### Drift Detection

```
AI Agent (scheduled task):

1. For each environment config:
   → execute_workflow(
       config={
         workspaces: [
           // All workspaces with plan-only operations
         ]
       }
     )

2. Parse plan outputs for changes
3. If changes detected:
   - Alert operations team
   - Generate drift report
   - Suggest remediation
```

### Cost Optimization

```
User: "Identify any infrastructure that could be optimized"

AI Agent:
1. List all workspaces across environments
   → list_workflows(config_path="*/infra.yaml")

2. Analyze resource configurations
3. Compare with cloud provider recommendations
4. Generate optimization report:
   - Underutilized resources
   - Right-sizing recommendations
   - Reserved instance opportunities
```

---

## Troubleshooting

### Common Issues

| Issue | Cause | Solution |
|-------|-------|----------|
| "Unable to create Temporal client" | Temporal server not running | Start Temporal: `temporal server start-dev` |
| "Config file not found" | Wrong path or missing file | Verify config_path is correct |
| "Unsupported workflow" | Invalid workflow_name | Use "ParentWorkflow" |
| "Invalid config" | Configuration validation failed | Check for cycles, missing deps |
| Timeout on execute_workflow | Long-running Terraform operations | Increase client timeout |

### Debugging

Enable verbose logging:

```bash
# Start MCP server with debug output
TEMPORAL_DEBUG=1 ./mcp-server 2>mcp-debug.log
```

Monitor Temporal Web UI:
- Open http://localhost:8233
- View workflow execution history
- Inspect activity inputs/outputs

---

*For more information, see the main [README.md](../README.md) and [ARCHITECTURE.md](./ARCHITECTURE.md).*
