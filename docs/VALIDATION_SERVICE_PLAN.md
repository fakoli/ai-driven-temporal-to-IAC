# TFVars Validation Service - Implementation Plan

## Overview

A new validation service that validates combined TFvars against CEL (Common Expression Language) rules before Terraform execution. This service sits between the orchestration layer and the intent-driven workflow, providing explicit feedback on configuration errors.

## Key Features

1. **CEL-based Rule Engine**: Validation rules written in CEL for flexibility and readability
2. **Rule File Storage**: Rules stored as `.cel` files in the project under `validation/rules/`
3. **Multiple Entry Points**: Standalone CLI, Temporal activity, and MCP tool
4. **Explicit Feedback**: Detailed error messages with remediation suggestions
5. **MCP Integration**: Returns incomplete validation status for AI agent workflows

---

## Architecture

### File Structure

```
/home/user/ai-driven-temporal-to-IAC/
├── cmd/
│   └── validator/                    # NEW: Standalone validation CLI
│       └── main.go
├── validation/                       # NEW: Validation service package
│   ├── service.go                   # Core validation service
│   ├── cel_engine.go                # CEL environment and custom functions
│   ├── rule_loader.go               # Rule file discovery and parsing
│   ├── types.go                     # Result types and structures
│   └── validators.go                # Built-in helper functions
├── validation/rules/                # NEW: CEL rule storage
│   ├── network/
│   │   ├── rfc1918.cel             # RFC 1918 private subnet validation
│   │   └── cidr.cel                # CIDR format validation
│   ├── common/
│   │   ├── required.cel            # Required field validation
│   │   └── string_length.cel       # String length constraints
│   └── README.md                   # Rule authoring documentation
├── activities/
│   └── validation_activities.go    # NEW: Temporal activity wrapper
└── workflow/
    └── validation.go               # NEW: Workflow integration helpers
```

### Integration Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                         CURRENT FLOW                              │
│  LoadConfig → ValidateConfig → NormalizeConfig → ExecuteWorkflow │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│                         NEW FLOW                                  │
│  LoadConfig → ValidateConfig → NormalizeConfig                   │
│      ↓                                                            │
│  CreateCombinedTFVars                                            │
│      ↓                                                            │
│  ┌─────────────────────────────────────────┐                     │
│  │        VALIDATION SERVICE (NEW)          │                     │
│  │  • Load CEL rules from files            │                     │
│  │  • Evaluate against combined tfvars     │                     │
│  │  • Return errors/warnings/info          │                     │
│  └─────────────────────────────────────────┘                     │
│      ↓                                                            │
│  If Valid → Continue to Terraform Operations                     │
│  If Invalid → Return detailed error to user/MCP                  │
└──────────────────────────────────────────────────────────────────┘
```

---

## Rule File Format

### CEL Rule Structure

Rules are stored as `.cel` files with metadata in comments:

```cel
# @target: private_subnet, vpc_cidr, subnet_cidr
# @severity: error
# @description: Private subnets must use RFC 1918 address space (10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
# @remediation: Use a valid RFC 1918 private IP range

has(vars.private_subnet) && vars.private_subnet != "" ?
  (vars.private_subnet.matches("^10\\..*") ||
   vars.private_subnet.matches("^172\\.(1[6-9]|2[0-9]|3[0-1])\\..*") ||
   vars.private_subnet.matches("^192\\.168\\..*")) :
  true
```

### Metadata Fields

| Field | Required | Description |
|-------|----------|-------------|
| `@target` | Yes | Comma-separated list of variable names this rule applies to |
| `@severity` | Yes | One of: `error`, `warning`, `info` |
| `@description` | Yes | Human-readable description of the validation |
| `@remediation` | No | Suggested fix when validation fails |

### CEL Expression Context

Rules have access to:
- `vars` - The complete combined tfvars map
- `workspace` - Workspace metadata (`name`, `kind`)
- Custom functions: `isRFC1918(string)`, `isCIDR(string)`, etc.

---

## Example Rules

### RFC 1918 Validation (`network/rfc1918.cel`)

```cel
# @target: private_subnet, vpc_cidr
# @severity: error
# @description: Private network addresses must be within RFC 1918 space
# @remediation: Use one of: 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16

has(vars.private_subnet) && vars.private_subnet != "" ?
  isRFC1918(vars.private_subnet) :
  true
```

### CIDR Format Validation (`network/cidr.cel`)

```cel
# @target: vpc_cidr, subnet_cidr
# @severity: error
# @description: Network addresses must be valid CIDR notation
# @remediation: Use format like 10.0.0.0/16

has(vars.vpc_cidr) && vars.vpc_cidr != "" ?
  vars.vpc_cidr.matches("^([0-9]{1,3}\\.){3}[0-9]{1,3}/[0-9]{1,2}$") :
  true
```

### Required Region (`common/required_region.cel`)

```cel
# @target: region
# @severity: error
# @description: AWS region is required for all workspaces
# @remediation: Set the 'region' variable (e.g., us-east-1)

has(vars.region) && vars.region != ""
```

### Availability Zones (`common/availability_zones.cel`)

```cel
# @target: availability_zones
# @severity: error
# @description: At least one availability zone must be specified
# @remediation: Provide a list of AZs (e.g., ["us-east-1a", "us-east-1b"])

has(vars.availability_zones) ? size(vars.availability_zones) > 0 : false
```

---

## Validation Result Format

### Go Types

```go
type ValidationResult struct {
    Valid      bool              `json:"valid"`
    Errors     []ValidationIssue `json:"errors"`
    Warnings   []ValidationIssue `json:"warnings"`
    Info       []ValidationIssue `json:"info"`
    Timestamp  time.Time         `json:"timestamp"`
}

type ValidationIssue struct {
    RuleID      string      `json:"rule_id"`       // e.g., "network.rfc1918"
    RuleName    string      `json:"rule_name"`     // e.g., "RFC 1918 Validation"
    Variable    string      `json:"variable"`      // e.g., "private_subnet"
    Value       interface{} `json:"value"`         // Actual failing value
    Message     string      `json:"message"`       // Human-readable error
    Severity    string      `json:"severity"`      // error, warning, info
    Remediation string      `json:"remediation"`   // Suggested fix
}
```

### Example MCP Response

```json
{
  "validation_status": "incomplete",
  "workspaces": {
    "vpc": {
      "valid": false,
      "errors": [
        {
          "rule_id": "network.rfc1918",
          "rule_name": "RFC 1918 Validation",
          "variable": "private_subnet",
          "value": "192.160.0.0/16",
          "message": "Private subnets must use RFC 1918 address space",
          "severity": "error",
          "remediation": "Use one of: 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16"
        }
      ],
      "warnings": []
    }
  },
  "summary": {
    "total_workspaces": 2,
    "valid_workspaces": 1,
    "failed_workspaces": 1,
    "total_errors": 1,
    "total_warnings": 0
  }
}
```

---

## MCP Tool Integration

### New Tool: `validate_tfvars`

```go
s.AddTool(mcp.NewTool("validate_tfvars",
    mcp.WithDescription("Validate Terraform variables against CEL rules before execution"),
    mcp.WithString("config_path", mcp.Description("Path to YAML config file"), mcp.Required()),
    mcp.WithString("workspace_name", mcp.Description("Specific workspace to validate (optional)")),
    mcp.WithBoolean("fail_on_warning", mcp.Description("Treat warnings as errors (default: false)")),
), validateTFVarsHandler)
```

### AI Agent Flow

```
1. User provides config via natural language
2. AI Agent calls validate_tfvars tool
3. If validation fails:
   - Return "incomplete" status with errors
   - Show user specific failures and remediations
   - User must fix before proceeding
4. If validation passes:
   - Proceed with execute_workflow
```

---

## Standalone CLI Usage

### Basic Commands

```bash
# Validate all workspaces
go run ./cmd/validator -config infra.yaml

# Validate specific workspace
go run ./cmd/validator -config infra.yaml -workspace vpc

# JSON output for automation
go run ./cmd/validator -config infra.yaml -format json

# Custom rules directory
go run ./cmd/validator -config infra.yaml -rules /path/to/rules

# Fail on warnings
go run ./cmd/validator -config infra.yaml -fail-on-warning
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All validations passed |
| 1 | One or more validation errors |
| 2 | Configuration or rule loading error |

---

## Implementation Phases

### Phase 1: Foundation (Core CEL Engine)
- [ ] Update `go.mod` with `github.com/google/cel-go`
- [ ] Create `validation/types.go` - Result structures
- [ ] Create `validation/cel_engine.go` - CEL environment setup
- [ ] Create `validation/validators.go` - Helper functions (isRFC1918, isCIDR)

### Phase 2: Rule Management
- [ ] Create `validation/rule_loader.go` - Rule discovery and parsing
- [ ] Create `validation/rules/` directory structure
- [ ] Implement rule metadata parsing from comments
- [ ] Create example rules (RFC 1918, CIDR, required fields)

### Phase 3: Service Layer
- [ ] Create `validation/service.go` - Core validation logic
- [ ] Implement `ValidateWorkspace()` method
- [ ] Implement `Validate()` method with rule execution
- [ ] Add comprehensive error formatting

### Phase 4: Temporal Integration
- [ ] Create `activities/validation_activities.go`
- [ ] Register validation activity in `cmd/worker/main.go`
- [ ] Create `workflow/validation.go` - Workflow helpers
- [ ] Modify `workflow/terraform_workflow.go` to call validation

### Phase 5: Standalone CLI
- [ ] Create `cmd/validator/main.go`
- [ ] Implement text and JSON output formats
- [ ] Add command-line flags
- [ ] Test standalone validation

### Phase 6: MCP Integration
- [ ] Add `validate_tfvars` tool to `cmd/mcp-server/main.go`
- [ ] Implement `validateTFVarsHandler()`
- [ ] Create structured JSON response format
- [ ] Add "incomplete validation" status handling

### Phase 7: Testing & Documentation
- [ ] Create unit tests for CEL rule evaluation
- [ ] Create integration tests for validation service
- [ ] Add test cases for each example rule
- [ ] Document rule creation in `validation/rules/README.md`
- [ ] Update main README with validation documentation

---

## Critical Files Summary

| File | Purpose |
|------|---------|
| `validation/service.go` | Core validation service with CEL integration |
| `validation/cel_engine.go` | CEL environment, custom functions |
| `validation/rule_loader.go` | Rule file discovery and parsing |
| `activities/validation_activities.go` | Temporal activity wrapper |
| `cmd/validator/main.go` | Standalone CLI entry point |
| `cmd/mcp-server/main.go` | MCP tool integration (modify) |
| `workflow/terraform_workflow.go` | Workflow integration point (modify) |

---

## Dependencies

### Go Packages to Add

```go
require (
    github.com/google/cel-go v0.20.0
    google.golang.org/genproto/googleapis/api v0.0.0-20231106174013-bbf56f31fb17
)
```

### Custom CEL Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `isRFC1918` | `(string) -> bool` | Validates RFC 1918 private IP range |
| `isCIDR` | `(string) -> bool` | Validates CIDR notation format |
| `isValidIP` | `(string) -> bool` | Validates IPv4 address format |
| `cidrContains` | `(string, string) -> bool` | Checks if CIDR contains IP |

---

## Open Questions

1. **Rule Precedence**: How should conflicting rules be handled?
2. **Workspace-Specific Rules**: Should rules be filterable by workspace kind (terraform, tofu)?
3. **Runtime vs Static**: Should some validations run after `terraform plan` output?
4. **Rule Versioning**: How to handle rule updates across deployments?
5. **Performance**: Should compiled rules be cached between invocations?

---

## User Story Example

**Scenario**: User attempts to deploy infrastructure with invalid private subnet

```yaml
# infra.yaml
workspaces:
  - name: vpc
    dir: terraform/vpc
    tfvars: vpc.tfvars.json
```

```json
// vpc.tfvars.json
{
  "private_subnet": "192.160.0.0/16",  // Invalid! Should be 192.168.x.x
  "region": "us-east-1"
}
```

**Via MCP (AI Agent)**:
```
AI: I'll validate your configuration before deploying...

[Calls validate_tfvars tool]

AI: ❌ Validation failed for workspace 'vpc':

   1. [private_subnet] RFC 1918 Validation
      Value: "192.160.0.0/16"
      Error: Private subnets must use RFC 1918 address space
      Fix: Use one of: 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16

Please update your private_subnet to a valid RFC 1918 range (e.g., 192.168.0.0/16).
```

**Via CLI**:
```bash
$ go run ./cmd/validator -config infra.yaml

Workspace: vpc
Status: FAILED

Errors:
  • [private_subnet] RFC 1918 Validation: Private subnets must use RFC 1918 address space
    Value: 192.160.0.0/16
    Fix: Use one of: 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16

Validation failed with 1 error(s)
$ echo $?
1
```
