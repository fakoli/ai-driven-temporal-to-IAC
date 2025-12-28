# System Architecture Documentation

This document provides detailed architectural diagrams and flow documentation for the Temporal Terraform Orchestrator.

## Table of Contents

1. [System Overview](#system-overview)
2. [Component Architecture](#component-architecture)
3. [Workflow Execution Flows](#workflow-execution-flows)
4. [Signal Communication Patterns](#signal-communication-patterns)
5. [Data Flow Diagrams](#data-flow-diagrams)
6. [Dependency Resolution](#dependency-resolution)
7. [Error Handling and Recovery](#error-handling-and-recovery)

---

## System Overview

### High-Level System Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                              ENTRY POINTS                                     │
│  ┌─────────────────────┐  ┌─────────────────────┐  ┌─────────────────────┐  │
│  │    CLI Starter      │  │    MCP Server       │  │   Direct API Call   │  │
│  │  (cmd/starter)      │  │  (cmd/mcp-server)   │  │   (Temporal Client) │  │
│  │                     │  │                     │  │                     │  │
│  │  Human operators    │  │  AI Agents          │  │  Custom integrations│  │
│  │  run deployments    │  │  (Claude, GPT, etc) │  │  and automation     │  │
│  └──────────┬──────────┘  └──────────┬──────────┘  └──────────┬──────────┘  │
│             │                        │                        │              │
│             └────────────────────────┼────────────────────────┘              │
│                                      │                                       │
│                                      ▼                                       │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                    CONFIGURATION LAYER                                 │  │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │  │
│  │  │ YAML Parser  │  │  Validator   │  │  Normalizer  │                 │  │
│  │  │              │  │              │  │              │                 │  │
│  │  │ infra.yaml   │──▶ Cycle check  │──▶ Path resolve │                 │  │
│  │  │ infra.json   │  │ Dep check    │  │ Defaults     │                 │  │
│  │  └──────────────┘  └──────────────┘  └──────────────┘                 │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                           TEMPORAL SERVER                                     │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                     WORKFLOW ORCHESTRATION                             │  │
│  │                                                                        │  │
│  │  ┌────────────────────────────────────────────────────────────────┐   │  │
│  │  │                    ParentWorkflow                               │   │  │
│  │  │  ┌─────────────────────────────────────────────────────────┐   │   │  │
│  │  │  │  1. Validate & Normalize Config                          │   │   │  │
│  │  │  │  2. Calculate DAG Depths                                 │   │   │  │
│  │  │  │  3. Start Root Workspaces (parallel)                     │   │   │  │
│  │  │  │  4. Listen for Completion Signals                        │   │   │  │
│  │  │  │  5. Trigger Ready Dependents                             │   │   │  │
│  │  │  │  6. Propagate Outputs as Inputs                          │   │   │  │
│  │  │  │  7. Signal Shutdown when Complete                        │   │   │  │
│  │  │  └─────────────────────────────────────────────────────────┘   │   │  │
│  │  └────────────────────────────────────────────────────────────────┘   │  │
│  │         │              │              │              │                 │  │
│  │         ▼              ▼              ▼              ▼                 │  │
│  │  ┌───────────┐  ┌───────────┐  ┌───────────┐  ┌───────────┐          │  │
│  │  │ Terraform │  │ Terraform │  │ Terraform │  │ Terraform │          │  │
│  │  │ Workflow  │  │ Workflow  │  │ Workflow  │  │ Workflow  │          │  │
│  │  │  (vpc)    │  │ (vpc-2)   │  │ (subnets) │  │  (eks)    │          │  │
│  │  └───────────┘  └───────────┘  └───────────┘  └───────────┘          │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
│                                                                              │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                         TEMPORAL WORKER                                │  │
│  │  ┌─────────────────────────────────────────────────────────────────┐  │  │
│  │  │                      Activities                                  │  │  │
│  │  │  ┌────────────┐ ┌────────────┐ ┌────────────┐ ┌────────────┐   │  │  │
│  │  │  │    Init    │ │  Validate  │ │    Plan    │ │   Apply    │   │  │  │
│  │  │  └────────────┘ └────────────┘ └────────────┘ └────────────┘   │  │  │
│  │  │  ┌────────────┐                                                 │  │  │
│  │  │  │   Output   │                                                 │  │  │
│  │  │  └────────────┘                                                 │  │  │
│  │  └─────────────────────────────────────────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                          TERRAFORM CLI                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │  terraform init    │  terraform validate  │  terraform plan           │  │
│  │  terraform apply   │  terraform output                                 │  │
│  └───────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────┘
                                       │
                                       ▼
┌──────────────────────────────────────────────────────────────────────────────┐
│                        CLOUD PROVIDERS                                        │
│  ┌────────────────┐  ┌────────────────┐  ┌────────────────┐                 │
│  │      AWS       │  │      GCP       │  │     Azure      │                 │
│  └────────────────┘  └────────────────┘  └────────────────┘                 │
└──────────────────────────────────────────────────────────────────────────────┘
```

---

## Component Architecture

### Component Interaction Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                              │
│    ┌──────────────────┐         ┌──────────────────┐                        │
│    │   infra.yaml     │         │   CLI Starter    │                        │
│    │                  │◀────────│   main.go        │                        │
│    │  - workspaces    │         │                  │                        │
│    │  - dependencies  │         │  Flags:          │                        │
│    │  - inputs        │         │  -config         │                        │
│    │  - operations    │         │  -task-queue     │                        │
│    └────────┬─────────┘         │  -workflow-id    │                        │
│             │                   └────────┬─────────┘                        │
│             │                            │                                   │
│             ▼                            ▼                                   │
│    ┌─────────────────────────────────────────────────┐                      │
│    │              workflow/config.go                  │                      │
│    │                                                  │                      │
│    │  LoadConfigFromFile()    - Parse YAML/JSON      │                      │
│    │  ValidateInfrastructureConfig()                 │                      │
│    │    - Check duplicates                           │                      │
│    │    - Detect cycles (DFS)                        │                      │
│    │    - Validate dependencies                      │                      │
│    │    - Validate input mappings                    │                      │
│    │    - Validate operations                        │                      │
│    │  NormalizeInfrastructureConfig()                │                      │
│    │    - Resolve paths                              │                      │
│    │    - Apply defaults                             │                      │
│    │  CalculateDepths()                              │                      │
│    │    - Compute DAG depths                         │                      │
│    └────────────────────┬────────────────────────────┘                      │
│                         │                                                    │
│                         ▼                                                    │
│    ┌─────────────────────────────────────────────────┐                      │
│    │          workflow/parent_workflow.go             │                      │
│    │                                                  │                      │
│    │  ParentWorkflow()                               │                      │
│    │    │                                            │                      │
│    │    ├── Start root workspaces (no deps)          │                      │
│    │    │                                            │                      │
│    │    ├── Orchestration loop:                      │                      │
│    │    │   ├── Listen for SignalWorkspaceFinished   │                      │
│    │    │   ├── Record outputs                       │                      │
│    │    │   ├── Check ready workspaces               │                      │
│    │    │   └── Start ready workspaces               │                      │
│    │    │                                            │                      │
│    │    ├── Signal shutdown to all                   │                      │
│    │    └── Wait for root futures                    │                      │
│    │                                                  │                      │
│    │  startWorkspace()                               │                      │
│    │    ├── Resolve input mappings                   │                      │
│    │    ├── Determine host (deepest dep)             │                      │
│    │    └── Signal host or start as root             │                      │
│    └────────────────────┬────────────────────────────┘                      │
│                         │                                                    │
│                         ▼                                                    │
│    ┌─────────────────────────────────────────────────┐                      │
│    │         workflow/terraform_workflow.go           │                      │
│    │                                                  │                      │
│    │  TerraformWorkflow()                            │                      │
│    │    │                                            │                      │
│    │    ├── Configure activity options               │                      │
│    │    │   (10min timeout, 3 retries)               │                      │
│    │    │                                            │                      │
│    │    ├── Execute operations in order:             │                      │
│    │    │   ├── init                                 │                      │
│    │    │   ├── validate                             │                      │
│    │    │   ├── plan (detect changes)                │                      │
│    │    │   └── apply (if changes present)           │                      │
│    │    │                                            │                      │
│    │    ├── Fetch outputs                            │                      │
│    │    ├── Signal parent with outputs               │                      │
│    │    │                                            │                      │
│    │    └── Hosting mode (if has parent):            │                      │
│    │        ├── Listen for SignalStartChild          │                      │
│    │        ├── Spawn child workflows                │                      │
│    │        └── Wait for SignalShutdown              │                      │
│    └────────────────────┬────────────────────────────┘                      │
│                         │                                                    │
│                         ▼                                                    │
│    ┌─────────────────────────────────────────────────┐                      │
│    │       activities/terraform_activities.go         │                      │
│    │                                                  │                      │
│    │  TerraformInit()     - terraform init           │                      │
│    │  TerraformValidate() - terraform validate       │                      │
│    │  TerraformPlan()     - terraform plan           │                      │
│    │                        -detailed-exitcode       │                      │
│    │  TerraformApply()    - terraform apply          │                      │
│    │  TerraformOutput()   - terraform output -json   │                      │
│    │                                                  │                      │
│    │  createCombinedTFVars()                         │                      │
│    │    - Parse original tfvars (HCL/JSON)           │                      │
│    │    - Merge with ExtraVars                       │                      │
│    │    - Write as JSON tfvars                       │                      │
│    └─────────────────────────────────────────────────┘                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Workflow Execution Flows

### Complete Execution Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        EXECUTION TIMELINE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  T0: User/AI triggers deployment                                             │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  CLI Starter / MCP Server                                                    │
│  │                                                                           │
│  ├── Load infra.yaml                                                         │
│  ├── Validate configuration                                                  │
│  ├── Normalize paths and defaults                                            │
│  └── Start ParentWorkflow via Temporal client                                │
│                                                                              │
│  T1: ParentWorkflow starts                                                   │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  ParentWorkflow                                                              │
│  │                                                                           │
│  ├── Calculate workspace depths                                              │
│  │   vpc: 0, vpc-2: 0, subnets: 1, eks: 2                                   │
│  │                                                                           │
│  ├── Start root workspaces (depth=0):                                        │
│  │   ├── Start TerraformWorkflow(vpc)    ──┐                                │
│  │   └── Start TerraformWorkflow(vpc-2)  ──┼── Parallel execution           │
│  │                                          │                                │
│  └── Enter orchestration loop               │                                │
│                                             │                                │
│  T2: Root workspaces execute               ▼                                │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  TerraformWorkflow(vpc)     │    TerraformWorkflow(vpc-2)                   │
│  │                          │    │                                          │
│  ├── terraform init         │    ├── terraform init                         │
│  ├── terraform validate     │    ├── terraform validate                     │
│  ├── terraform plan         │    ├── terraform plan                         │
│  ├── terraform apply        │    └── (plan only - no apply)                 │
│  ├── terraform output       │                                               │
│  │   {vpc_id: "vpc-123"}    │                                               │
│  │                          │                                                │
│  ├── Signal: WorkspaceFinished(vpc, outputs)                                │
│  └── Enter hosting mode     │                                                │
│                             │                                                │
│  T3: Dependent workspace triggered                                           │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  ParentWorkflow receives signal                                              │
│  │                                                                           │
│  ├── Record: vpc completed with outputs                                      │
│  ├── Check: subnets dependencies met? Yes (vpc done)                        │
│  ├── Resolve inputs: subnets.vpc_id = vpc.outputs.vpc_id                    │
│  └── Signal: StartChild(subnets) to vpc workflow (deepest dep)              │
│                                                                              │
│  TerraformWorkflow(vpc) - hosting mode                                       │
│  │                                                                           │
│  ├── Receive: StartChild(subnets)                                           │
│  └── Start TerraformWorkflow(subnets) as child                              │
│                                                                              │
│  T4: Subnets workspace executes                                              │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  TerraformWorkflow(subnets)                                                  │
│  │                                                                           │
│  ├── createCombinedTFVars()                                                  │
│  │   Original: subnets.tfvars                                               │
│  │   + ExtraVars: {vpc_id: "vpc-123"}                                       │
│  │   = combined.tfvars.json                                                  │
│  │                                                                           │
│  ├── terraform init                                                          │
│  ├── terraform validate                                                      │
│  ├── terraform plan -var-file=combined.tfvars.json                          │
│  ├── terraform apply                                                         │
│  ├── terraform output                                                        │
│  │   {subnet_ids: ["subnet-a", "subnet-b"]}                                 │
│  │                                                                           │
│  ├── Signal: WorkspaceFinished(subnets, outputs)                            │
│  └── Enter hosting mode                                                      │
│                                                                              │
│  T5: EKS workspace triggered                                                 │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  ParentWorkflow receives signal                                              │
│  │                                                                           │
│  ├── Record: subnets completed with outputs                                  │
│  ├── Check: eks dependencies met? Yes (vpc + subnets done)                  │
│  ├── Resolve inputs:                                                         │
│  │   eks.vpc_id = vpc.outputs.vpc_id                                        │
│  │   eks.subnet_ids = subnets.outputs.subnet_ids                            │
│  └── Signal: StartChild(eks) to subnets workflow (deepest dep)             │
│                                                                              │
│  TerraformWorkflow(subnets) - hosting mode                                   │
│  │                                                                           │
│  ├── Receive: StartChild(eks)                                               │
│  └── Start TerraformWorkflow(eks) as child                                  │
│                                                                              │
│  T6: EKS workspace executes                                                  │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  TerraformWorkflow(eks)                                                      │
│  │                                                                           │
│  ├── createCombinedTFVars()                                                  │
│  │   Original: eks.tfvars                                                   │
│  │   + ExtraVars: {vpc_id: "vpc-123", subnet_ids: [...]}                   │
│  │   = combined.tfvars.json                                                  │
│  │                                                                           │
│  ├── terraform init                                                          │
│  ├── terraform validate                                                      │
│  ├── terraform plan -var-file=combined.tfvars.json                          │
│  └── (plan only - no apply in this config)                                  │
│                                                                              │
│  ├── Signal: WorkspaceFinished(eks, outputs)                                │
│  └── Exit (no children to host)                                             │
│                                                                              │
│  T7: Orchestration completes                                                 │
│  ══════════════════════════════════════════════════════════════════════════ │
│                                                                              │
│  ParentWorkflow                                                              │
│  │                                                                           │
│  ├── All workspaces completed                                               │
│  ├── Signal: Shutdown to all running workflows                              │
│  │   ├── vpc receives shutdown, exits hosting mode                          │
│  │   └── subnets receives shutdown, exits hosting mode                      │
│  │                                                                           │
│  ├── Wait for all root futures                                              │
│  └── Return success                                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Parallel Execution Visualization

```
Time ──────────────────────────────────────────────────────────────────────────▶

     ┌──────────────────────────────────────────────────────────────────────┐
     │                         ParentWorkflow                                │
     └──────────────────────────────────────────────────────────────────────┘
          │                                                              │
          │ Start                                                   Wait │
          ▼                                                              ▼
     ┌─────────────┐
     │    vpc      │ ═══════════════════════╗
     │ init→plan→  │                        ║ Signal: finished
     │ apply→output│                        ║ {vpc_id: "vpc-123"}
     └─────────────┘                        ║
                                            ║
     ┌─────────────┐                        ║
     │   vpc-2     │ ════════╗              ║
     │ init→plan   │         ║              ║
     │ (no apply)  │         ║              ║
     └─────────────┘         ║              ║
                             ║              ║
                             ║              ▼
                             ║         ┌─────────────┐
                             ║         │  subnets    │ ═══════════════╗
                             ║         │ init→plan→  │                ║
                             ║         │ apply→output│                ║
                             ║         └─────────────┘                ║
                             ║              │                         ║
                             ║              │ Receives vpc_id        ║
                             ║              │ from vpc outputs       ║
                             ║                                        ║
                             ║                                        ▼
                             ║                                   ┌─────────────┐
                             ║                                   │    eks      │
                             ║                                   │ init→plan   │
                             ║                                   │ (no apply)  │
                             ║                                   └─────────────┘
                             ║                                        │
                             ║         Receives vpc_id + subnet_ids   │
                             ║                                        │
     ════════════════════════╩════════════════════════════════════════╧════════▶

     Legend:
     ═══  Workflow execution
     ───▶ Time progression
     │    Signal/data flow
```

---

## Signal Communication Patterns

### Signal Types and Payloads

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          SIGNAL DEFINITIONS                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SignalWorkspaceFinished                                                     │
│  ════════════════════════                                                    │
│  Direction: TerraformWorkflow → ParentWorkflow                              │
│  Purpose: Report workspace completion with outputs                           │
│                                                                              │
│  Payload:                                                                    │
│  ┌─────────────────────────────────────────────┐                            │
│  │  WorkspaceFinishedSignal {                  │                            │
│  │    Name:    string                          │  // "vpc"                  │
│  │    Outputs: map[string]interface{}          │  // {vpc_id: "vpc-123"}   │
│  │  }                                          │                            │
│  └─────────────────────────────────────────────┘                            │
│                                                                              │
│  SignalStartChild                                                            │
│  ════════════════                                                            │
│  Direction: ParentWorkflow → TerraformWorkflow (host)                       │
│  Purpose: Request host workflow to spawn a child                             │
│                                                                              │
│  Payload:                                                                    │
│  ┌─────────────────────────────────────────────┐                            │
│  │  StartChildSignal {                         │                            │
│  │    Workspace: WorkspaceConfig {             │                            │
│  │      Name:      string                      │  // "subnets"              │
│  │      Dir:       string                      │  // "/path/to/subnets"     │
│  │      ExtraVars: map[string]interface{}      │  // Resolved inputs        │
│  │      ...                                    │                            │
│  │    }                                        │                            │
│  │  }                                          │                            │
│  └─────────────────────────────────────────────┘                            │
│                                                                              │
│  SignalShutdown                                                              │
│  ══════════════                                                              │
│  Direction: ParentWorkflow → All TerraformWorkflows                         │
│  Purpose: Signal workflows to exit hosting mode                              │
│                                                                              │
│  Payload: nil (no data needed)                                              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Signal Flow Diagram

```
┌───────────────────────────────────────────────────────────────────────────┐
│                                                                            │
│   ParentWorkflow                                                           │
│   ┌────────────────────────────────────────────────────────────────────┐  │
│   │                                                                     │  │
│   │   finishedChan = GetSignalChannel(SignalWorkspaceFinished)         │  │
│   │                                                                     │  │
│   │   for len(completed) < len(workspaces) {                           │  │
│   │       selector.AddReceive(finishedChan, handleFinished)            │  │
│   │       selector.Select(ctx)  // Block until signal received         │  │
│   │   }                                                                 │  │
│   │                                                                     │  │
│   └────────────────────────────────────────────────────────────────────┘  │
│        ▲                           │                                       │
│        │ SignalWorkspaceFinished   │ SignalStartChild                     │
│        │                           ▼                                       │
│   ┌────┴───────────────────────────────────────────────────────────────┐  │
│   │                                                                     │  │
│   │   TerraformWorkflow (vpc) - Host                                   │  │
│   │   ┌─────────────────────────────────────────────────────────────┐  │  │
│   │   │                                                              │  │  │
│   │   │   // After terraform operations complete                     │  │  │
│   │   │   SignalExternalWorkflow(orchestratorID,                     │  │  │
│   │   │                          SignalWorkspaceFinished,            │  │  │
│   │   │                          {Name: "vpc", Outputs: {...}})      │  │  │
│   │   │                                                              │  │  │
│   │   │   // Hosting mode                                            │  │  │
│   │   │   childChannel = GetSignalChannel(SignalStartChild)          │  │  │
│   │   │   shutdownChannel = GetSignalChannel(SignalShutdown)         │  │  │
│   │   │                                                              │  │  │
│   │   │   for {                                                      │  │  │
│   │   │       selector.AddReceive(childChannel, spawnChild)          │  │  │
│   │   │       selector.AddReceive(shutdownChannel, markShutdown)     │  │  │
│   │   │       selector.Select(ctx)                                   │  │  │
│   │   │       if shouldShutdown && activeChildren == 0 { break }     │  │  │
│   │   │   }                                                          │  │  │
│   │   │                                                              │  │  │
│   │   └─────────────────────────────────────────────────────────────┘  │  │
│   │        │                                                            │  │
│   │        │ ExecuteChildWorkflow                                       │  │
│   │        ▼                                                            │  │
│   │   ┌─────────────────────────────────────────────────────────────┐  │  │
│   │   │   TerraformWorkflow (subnets) - Child                       │  │  │
│   │   │   ...signals parent when done...                            │  │  │
│   │   └─────────────────────────────────────────────────────────────┘  │  │
│   │                                                                     │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                            │
└───────────────────────────────────────────────────────────────────────────┘
```

---

## Data Flow Diagrams

### Output Propagation Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        OUTPUT PROPAGATION                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   VPC Workspace                                                              │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  terraform/vpc/main.tf                                               │   │
│   │  ┌───────────────────────────────────────────────────────────────┐  │   │
│   │  │  output "vpc_id" {                                             │  │   │
│   │  │    value = aws_vpc.main.id                                     │  │   │
│   │  │  }                                                             │  │   │
│   │  └───────────────────────────────────────────────────────────────┘  │   │
│   │                                                                      │   │
│   │  terraform output -json                                              │   │
│   │  ┌───────────────────────────────────────────────────────────────┐  │   │
│   │  │  {"vpc_id": {"value": "vpc-0123456789abcdef"}}                 │  │   │
│   │  └───────────────────────────────────────────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                           │                                                  │
│                           │ TerraformOutput activity parses JSON             │
│                           │ Extracts: {vpc_id: "vpc-0123456789abcdef"}      │
│                           ▼                                                  │
│   ParentWorkflow                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  workspaceOutputs["vpc"] = {vpc_id: "vpc-0123456789abcdef"}         │   │
│   │                                                                      │   │
│   │  // When starting subnets workspace:                                 │   │
│   │  for _, mapping := range ws.Inputs {                                 │   │
│   │      sourceOuts := workspaceOutputs[mapping.SourceWorkspace]        │   │
│   │      ws.ExtraVars[mapping.TargetVar] = sourceOuts[mapping.SourceOutput]│ │
│   │  }                                                                   │   │
│   │                                                                      │   │
│   │  Result: subnets.ExtraVars = {vpc_id: "vpc-0123456789abcdef"}       │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                           │                                                  │
│                           ▼                                                  │
│   Subnets Workspace                                                          │
│   ┌─────────────────────────────────────────────────────────────────────┐   │
│   │  createCombinedTFVars()                                              │   │
│   │                                                                      │   │
│   │  Original: terraform/subnets/subnets.tfvars                         │   │
│   │  ┌───────────────────────────────────────────────────────────────┐  │   │
│   │  │  region = "us-west-2"                                          │  │   │
│   │  │  availability_zones = ["us-west-2a", "us-west-2b"]            │  │   │
│   │  └───────────────────────────────────────────────────────────────┘  │   │
│   │                                                                      │   │
│   │  + ExtraVars: {vpc_id: "vpc-0123456789abcdef"}                      │   │
│   │                                                                      │   │
│   │  = Combined: /tmp/terraform-orchestrator/<runid>/combined.tfvars.json│  │
│   │  ┌───────────────────────────────────────────────────────────────┐  │   │
│   │  │  {                                                             │  │   │
│   │  │    "region": "us-west-2",                                      │  │   │
│   │  │    "availability_zones": ["us-west-2a", "us-west-2b"],        │  │   │
│   │  │    "vpc_id": "vpc-0123456789abcdef"                           │  │   │
│   │  │  }                                                             │  │   │
│   │  └───────────────────────────────────────────────────────────────┘  │   │
│   │                                                                      │   │
│   │  terraform plan -var-file=/tmp/.../combined.tfvars.json             │   │
│   │                                                                      │   │
│   │  terraform/subnets/main.tf                                          │   │
│   │  ┌───────────────────────────────────────────────────────────────┐  │   │
│   │  │  variable "vpc_id" {                                           │  │   │
│   │  │    type = string                                               │  │   │
│   │  │  }                                                             │  │   │
│   │  │                                                                │  │   │
│   │  │  resource "aws_subnet" "main" {                                │  │   │
│   │  │    vpc_id = var.vpc_id  // Uses "vpc-0123456789abcdef"        │  │   │
│   │  │    ...                                                         │  │   │
│   │  │  }                                                             │  │   │
│   │  └───────────────────────────────────────────────────────────────┘  │   │
│   └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Type Preservation Through the System

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    TYPE PRESERVATION FLOW                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Terraform Output (JSON)              Go Runtime                JSON TFVars │
│  ═══════════════════════              ══════════                ════════════│
│                                                                              │
│  String:                                                                     │
│  {"vpc_id":{"value":"vpc-123"}}  →  string("vpc-123")    →  "vpc-123"      │
│                                                                              │
│  Number:                                                                     │
│  {"count":{"value":42}}          →  float64(42)          →  42             │
│                                                                              │
│  Boolean:                                                                    │
│  {"enabled":{"value":true}}      →  bool(true)           →  true           │
│                                                                              │
│  Array:                                                                      │
│  {"ids":{"value":["a","b"]}}     →  []interface{}{"a","b"} → ["a","b"]     │
│                                                                              │
│  Object:                                                                     │
│  {"config":{"value":             →  map[string]interface{} → {"key":"val"} │
│    {"key":"val"}}}                    {"key":"val"}                         │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐│
│  │  The system preserves types throughout the pipeline:                   ││
│  │                                                                         ││
│  │  1. terraform output -json produces typed JSON                         ││
│  │  2. TerraformOutput activity parses into interface{}                   ││
│  │  3. ParentWorkflow stores in map[string]interface{}                    ││
│  │  4. Input mapping copies values preserving types                       ││
│  │  5. createCombinedTFVars() writes as JSON (preserves types)           ││
│  │  6. Terraform reads JSON tfvars with correct types                     ││
│  └────────────────────────────────────────────────────────────────────────┘│
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Dependency Resolution

### DAG Construction Algorithm

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      DEPENDENCY DAG CONSTRUCTION                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Input Configuration:                                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  workspaces:                                                         │   │
│  │    - name: vpc                                                       │   │
│  │      dependsOn: []                                                   │   │
│  │                                                                      │   │
│  │    - name: vpc-2                                                     │   │
│  │      dependsOn: []                                                   │   │
│  │                                                                      │   │
│  │    - name: subnets                                                   │   │
│  │      dependsOn: [vpc]                                                │   │
│  │                                                                      │   │
│  │    - name: eks                                                       │   │
│  │      dependsOn: [vpc, subnets]                                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Step 1: Build Adjacency List                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  vpc     → []                                                        │   │
│  │  vpc-2   → []                                                        │   │
│  │  subnets → [vpc]                                                     │   │
│  │  eks     → [vpc, subnets]                                            │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Step 2: Cycle Detection (DFS)                                               │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  visiting = {}  // Currently in DFS stack                            │   │
│  │  visited = {}   // Completed DFS                                     │   │
│  │                                                                      │   │
│  │  dfs(vpc):     visiting={vpc}     → visited={vpc}                   │   │
│  │  dfs(vpc-2):   visiting={vpc-2}   → visited={vpc,vpc-2}             │   │
│  │  dfs(subnets): visiting={subnets} → dfs(vpc) [already visited]      │   │
│  │                → visited={vpc,vpc-2,subnets}                         │   │
│  │  dfs(eks):     visiting={eks}     → dfs(vpc) [visited]              │   │
│  │                                   → dfs(subnets) [visited]           │   │
│  │                → visited={vpc,vpc-2,subnets,eks}                     │   │
│  │                                                                      │   │
│  │  Result: No cycles detected ✓                                        │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Step 3: Calculate Depths                                                    │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  getDepth(vpc)     = 0  (no dependencies)                           │   │
│  │  getDepth(vpc-2)   = 0  (no dependencies)                           │   │
│  │  getDepth(subnets) = max(getDepth(vpc)) + 1 = 0 + 1 = 1             │   │
│  │  getDepth(eks)     = max(getDepth(vpc), getDepth(subnets)) + 1      │   │
│  │                    = max(0, 1) + 1 = 2                               │   │
│  │                                                                      │   │
│  │  depths = {vpc: 0, vpc-2: 0, subnets: 1, eks: 2}                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Resulting DAG:                                                              │
│                                                                              │
│         Depth 0           Depth 1           Depth 2                         │
│         ═══════           ═══════           ═══════                         │
│                                                                              │
│        ┌─────┐                                                               │
│        │ vpc │────────────┐                                                  │
│        └─────┘            │                                                  │
│            │              ▼                                                  │
│            │         ┌─────────┐                                            │
│            │         │ subnets │────────┐                                   │
│            │         └─────────┘        │                                   │
│            │              │             ▼                                   │
│            │              │        ┌─────────┐                              │
│            └──────────────┼───────▶│   eks   │                              │
│                           │        └─────────┘                              │
│                           │             ▲                                   │
│                           └─────────────┘                                   │
│                                                                              │
│        ┌───────┐                                                             │
│        │ vpc-2 │  (independent, runs parallel with vpc)                     │
│        └───────┘                                                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Hosting Hierarchy Decision

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       HOSTING HIERARCHY                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Rule: A workspace is hosted by its DEEPEST dependency                       │
│                                                                              │
│  For workspace "eks" with dependsOn: [vpc, subnets]                         │
│    - depth(vpc) = 0                                                         │
│    - depth(subnets) = 1                                                     │
│    - Host = subnets (deeper)                                                │
│                                                                              │
│  Workflow Hierarchy:                                                         │
│                                                                              │
│  ParentWorkflow                                                              │
│  ├── TerraformWorkflow(vpc)      [root - depth 0]                           │
│  │   └── TerraformWorkflow(subnets)  [child - depth 1]                      │
│  │       └── TerraformWorkflow(eks)      [child - depth 2]                  │
│  │                                                                          │
│  └── TerraformWorkflow(vpc-2)    [root - depth 0, parallel]                 │
│                                                                              │
│  Why this matters:                                                           │
│  1. Creates logical grouping matching dependency structure                   │
│  2. Parent-child relationship in Temporal UI                                │
│  3. Proper workflow completion ordering                                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Error Handling and Recovery

### Retry Policy Configuration

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         RETRY POLICY                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Activity Options:                                                           │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  StartToCloseTimeout: 10 * time.Minute                               │   │
│  │                                                                      │   │
│  │  RetryPolicy: {                                                      │   │
│  │    MaximumAttempts:    3                                             │   │
│  │    InitialInterval:    5 * time.Second                               │   │
│  │    BackoffCoefficient: 2.0                                           │   │
│  │    MaximumInterval:    1 * time.Minute                               │   │
│  │  }                                                                   │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                              │
│  Retry Timeline:                                                             │
│                                                                              │
│  Attempt 1        Attempt 2        Attempt 3        Fail                    │
│  ─────────────────────────────────────────────────────────▶                 │
│      │                │                │                │                    │
│      │    5 sec       │    10 sec      │                │                    │
│      │◀──────────────▶│◀──────────────▶│                │                    │
│      ▼                ▼                ▼                ▼                    │
│  ┌───────┐        ┌───────┐        ┌───────┐        ┌───────┐              │
│  │ Fail  │        │ Fail  │        │ Fail  │        │ Error │              │
│  └───────┘        └───────┘        └───────┘        │Returned│              │
│                                                      └───────┘              │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Failure Scenarios

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       FAILURE SCENARIOS                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Scenario 1: Transient API Failure                                           │
│  ═══════════════════════════════════                                         │
│                                                                              │
│  terraform apply                                                             │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────────┐                                                    │
│  │ AWS API rate limit  │                                                    │
│  │ Error (attempt 1)   │                                                    │
│  └─────────────────────┘                                                    │
│      │                                                                       │
│      │ Wait 5 seconds (InitialInterval)                                     │
│      ▼                                                                       │
│  ┌─────────────────────┐                                                    │
│  │ Retry (attempt 2)   │                                                    │
│  │ Success ✓           │                                                    │
│  └─────────────────────┘                                                    │
│                                                                              │
│  Scenario 2: Permanent Failure                                               │
│  ═════════════════════════════                                               │
│                                                                              │
│  terraform validate                                                          │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────────┐                                                    │
│  │ Invalid HCL syntax  │  ← Non-retryable error (configuration issue)      │
│  │ Error (attempt 1)   │                                                    │
│  └─────────────────────┘                                                    │
│      │                                                                       │
│      │ Retry policy applies but error persists                              │
│      ▼                                                                       │
│  ┌─────────────────────┐                                                    │
│  │ MaximumAttempts     │                                                    │
│  │ reached (3)         │                                                    │
│  │ Workflow fails      │                                                    │
│  └─────────────────────┘                                                    │
│      │                                                                       │
│      ▼                                                                       │
│  ParentWorkflow receives error                                               │
│  Other workspaces continue (isolation)                                       │
│  Final result: Partial success + error details                               │
│                                                                              │
│  Scenario 3: Worker Crash                                                    │
│  ════════════════════════                                                    │
│                                                                              │
│  TerraformWorkflow executing                                                 │
│      │                                                                       │
│      ▼                                                                       │
│  ┌─────────────────────┐                                                    │
│  │ Worker process      │                                                    │
│  │ crashes mid-apply   │                                                    │
│  └─────────────────────┘                                                    │
│      │                                                                       │
│      │ Temporal detects worker heartbeat failure                            │
│      ▼                                                                       │
│  ┌─────────────────────┐                                                    │
│  │ Activity rescheduled│                                                    │
│  │ to another worker   │                                                    │
│  │ (if available)      │                                                    │
│  └─────────────────────┘                                                    │
│      │                                                                       │
│      ▼                                                                       │
│  Workflow continues from last checkpoint                                     │
│  (Temporal's durable execution guarantee)                                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## File Reference

| Component | File | Purpose |
|-----------|------|---------|
| CLI Entry | `cmd/starter/main.go` | Human-operated deployment trigger |
| Worker | `cmd/worker/main.go` | Temporal worker process |
| MCP Server | `cmd/mcp-server/main.go` | AI agent interface |
| Config | `workflow/config.go` | Types, validation, normalization |
| Orchestrator | `workflow/parent_workflow.go` | Main coordination workflow |
| Executor | `workflow/terraform_workflow.go` | Per-workspace execution |
| Activities | `activities/terraform_activities.go` | Terraform CLI wrappers |
| Constants | `utils/constants.go` | Shared configuration |

---

*Generated for Temporal Terraform Orchestrator - Intent-Driven Infrastructure Management*
