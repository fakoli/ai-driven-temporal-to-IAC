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
│   ├── common/                     # Shared rules (lowest precedence)
│   │   ├── 00_required_region.cel  # Required region field
│   │   ├── 01_valid_region.cel     # Valid AWS region format
│   │   └── 02_tags.cel             # Tag requirements
│   ├── vpc/                        # VPC workspace rules
│   │   ├── 00_cidr_format.cel      # CIDR format validation
│   │   ├── 01_rfc1918.cel          # RFC 1918 compliance
│   │   ├── 02_cidr_size.cel        # VPC CIDR size constraints
│   │   └── 03_dns_settings.cel     # DNS configuration
│   ├── subnets/                    # Subnet workspace rules
│   │   ├── 00_vpc_id_required.cel  # VPC ID dependency
│   │   ├── 01_cidr_rfc1918.cel     # Subnet CIDR RFC 1918
│   │   ├── 02_azs_required.cel     # Availability zones
│   │   ├── 03_subnet_sizing.cel    # Subnet size validation
│   │   └── 04_no_overlap.cel       # No CIDR overlap
│   ├── eks/                        # EKS workspace rules
│   │   ├── 00_dependencies.cel     # Required vpc_id, subnet_ids
│   │   ├── 01_cluster_name.cel     # Cluster naming conventions
│   │   ├── 02_version.cel          # K8s version validation
│   │   ├── 03_node_sizing.cel      # Node group constraints
│   │   └── 04_subnet_count.cel     # Min subnet requirements
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
| `@workspace` | No | Workspace name/pattern to apply rule (e.g., `vpc`, `eks`, `*`) |
| `@order` | No | Explicit ordering (default: filename sort order) |

### Rule Precedence Model

Rules are evaluated in a specific order, with **later rules having higher precedence**:

```
1. common/*.cel      → Lowest precedence (base rules)
2. {workspace}/*.cel → Workspace-specific rules (override common)
3. Within directory  → Alphabetical by filename (00_ before 01_)
4. @order metadata   → Explicit override if specified
```

**Conflict Resolution**: When multiple rules validate the same variable:
- All rules are evaluated
- Errors from ANY rule cause validation failure
- If workspace-specific rule passes but common rule fails, the failure stands
- Use `@order` to explicitly control evaluation sequence when needed

**Example**: If `common/01_valid_region.cel` and `vpc/99_override_region.cel` both validate `region`:
- Both rules execute
- `vpc/99_override_region.cel` has higher precedence (evaluated last)
- If the VPC rule explicitly returns `true`, it does NOT suppress the common rule's error
- To override, the VPC rule must handle the same validation logic

### CEL Expression Context

Rules have access to:
- `vars` - The complete combined tfvars map
- `workspace.name` - Current workspace name (e.g., "vpc", "eks")
- `workspace.kind` - Workspace kind (e.g., "terraform", "tofu")
- `workspace.dir` - Workspace directory path
- Custom functions: `isRFC1918(string)`, `isCIDR(string)`, etc.

---

## Example Rules

### Common Rules (Apply to All Workspaces)

#### `common/00_required_region.cel`
```cel
# @target: region
# @severity: error
# @workspace: *
# @description: AWS region is required for all workspaces
# @remediation: Set the 'region' variable (e.g., us-east-1, us-west-2)

has(vars.region) && vars.region != ""
```

#### `common/01_valid_region.cel`
```cel
# @target: region
# @severity: error
# @workspace: *
# @description: AWS region must be a valid region code
# @remediation: Use a valid AWS region (e.g., us-east-1, eu-west-1, ap-southeast-1)

has(vars.region) ?
  vars.region.matches("^(us|eu|ap|sa|ca|me|af)-(north|south|east|west|central|northeast|southeast)-[1-3]$") :
  true
```

#### `common/02_tags.cel`
```cel
# @target: tags
# @severity: warning
# @workspace: *
# @description: Resources should have standard tags for cost tracking and ownership
# @remediation: Add tags: Environment, Project, Owner, CostCenter

has(vars.tags) && type(vars.tags) == map ?
  (has(vars.tags.Environment) && has(vars.tags.Project)) :
  true  # Pass if no tags defined (not all workspaces use tags)
```

---

### VPC Workspace Rules

#### `vpc/00_cidr_format.cel`
```cel
# @target: vpc_cidr, cidr_block
# @severity: error
# @workspace: vpc
# @description: VPC CIDR must be valid CIDR notation
# @remediation: Use format like 10.0.0.0/16 or 172.16.0.0/12

has(vars.vpc_cidr) && vars.vpc_cidr != "" ?
  isCIDR(vars.vpc_cidr) :
  (has(vars.cidr_block) && vars.cidr_block != "" ? isCIDR(vars.cidr_block) : true)
```

#### `vpc/01_rfc1918.cel`
```cel
# @target: vpc_cidr, cidr_block
# @severity: error
# @workspace: vpc
# @description: VPC CIDR must use RFC 1918 private address space
# @remediation: Use one of: 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16

// Get the CIDR value from either vpc_cidr or cidr_block
has(vars.vpc_cidr) && vars.vpc_cidr != "" ?
  isRFC1918(vars.vpc_cidr) :
  (has(vars.cidr_block) && vars.cidr_block != "" ? isRFC1918(vars.cidr_block) : true)
```

#### `vpc/02_cidr_size.cel`
```cel
# @target: vpc_cidr, cidr_block
# @severity: error
# @workspace: vpc
# @description: VPC CIDR block must be between /16 and /28 (AWS limit)
# @remediation: Use a CIDR between /16 (65,536 IPs) and /28 (16 IPs). Recommended: /16 for production

has(vars.vpc_cidr) && vars.vpc_cidr != "" ?
  (int(vars.vpc_cidr.split("/")[1]) >= 16 && int(vars.vpc_cidr.split("/")[1]) <= 28) :
  true
```

#### `vpc/03_dns_settings.cel`
```cel
# @target: enable_dns_support, enable_dns_hostnames
# @severity: warning
# @workspace: vpc
# @description: DNS support and hostnames should be enabled for most use cases
# @remediation: Set enable_dns_support=true and enable_dns_hostnames=true

// Both should be true for EKS and most AWS services
has(vars.enable_dns_support) && has(vars.enable_dns_hostnames) ?
  (vars.enable_dns_support == true && vars.enable_dns_hostnames == true) :
  true  // Pass if not specified (Terraform defaults are fine)
```

#### `vpc/04_nat_gateway.cel`
```cel
# @target: enable_nat_gateway, single_nat_gateway, one_nat_gateway_per_az
# @severity: warning
# @workspace: vpc
# @description: Production VPCs should have NAT gateway configuration for private subnets
# @remediation: Set enable_nat_gateway=true. For HA, set one_nat_gateway_per_az=true

has(vars.enable_nat_gateway) ?
  vars.enable_nat_gateway == true :
  true  // Not required, just a recommendation
```

---

### Subnet Workspace Rules

#### `subnets/00_vpc_id_required.cel`
```cel
# @target: vpc_id
# @severity: error
# @workspace: subnets
# @description: VPC ID is required to create subnets
# @remediation: Ensure vpc_id is passed from the VPC workspace via input mapping

has(vars.vpc_id) && vars.vpc_id != "" && vars.vpc_id != "null"
```

#### `subnets/01_cidr_rfc1918.cel`
```cel
# @target: private_subnet_cidrs, public_subnet_cidrs, private_subnets, public_subnets
# @severity: error
# @workspace: subnets
# @description: All subnet CIDRs must use RFC 1918 private address space
# @remediation: Use subnets within 10.0.0.0/8, 172.16.0.0/12, or 192.168.0.0/16

// Validate private subnet CIDRs
has(vars.private_subnet_cidrs) && size(vars.private_subnet_cidrs) > 0 ?
  vars.private_subnet_cidrs.all(cidr, isRFC1918(cidr)) :
  (has(vars.private_subnets) && size(vars.private_subnets) > 0 ?
    vars.private_subnets.all(cidr, isRFC1918(cidr)) : true)
```

#### `subnets/02_azs_required.cel`
```cel
# @target: availability_zones, azs
# @severity: error
# @workspace: subnets
# @description: At least 2 availability zones required for high availability
# @remediation: Provide at least 2 AZs (e.g., ["us-east-1a", "us-east-1b"])

has(vars.availability_zones) ?
  size(vars.availability_zones) >= 2 :
  (has(vars.azs) ? size(vars.azs) >= 2 : false)
```

#### `subnets/03_subnet_sizing.cel`
```cel
# @target: private_subnet_cidrs, public_subnet_cidrs
# @severity: warning
# @workspace: subnets
# @description: Subnets should be appropriately sized (/19 to /28)
# @remediation: Use /24 for most workloads, /19-/20 for large clusters

// Check that subnet CIDRs are reasonable sizes
has(vars.private_subnet_cidrs) && size(vars.private_subnet_cidrs) > 0 ?
  vars.private_subnet_cidrs.all(cidr,
    int(cidr.split("/")[1]) >= 19 && int(cidr.split("/")[1]) <= 28
  ) : true
```

#### `subnets/04_no_overlap.cel`
```cel
# @target: private_subnet_cidrs, public_subnet_cidrs
# @severity: error
# @workspace: subnets
# @description: Subnet CIDRs must not overlap with each other
# @remediation: Ensure each subnet has a unique, non-overlapping CIDR block

// This is a complex check - using a helper function
has(vars.private_subnet_cidrs) && has(vars.public_subnet_cidrs) ?
  !cidrsOverlap(vars.private_subnet_cidrs, vars.public_subnet_cidrs) :
  true
```

#### `subnets/05_public_private_balance.cel`
```cel
# @target: private_subnet_cidrs, public_subnet_cidrs
# @severity: warning
# @workspace: subnets
# @description: Should have matching number of public and private subnets per AZ
# @remediation: Create one public and one private subnet per availability zone

has(vars.private_subnet_cidrs) && has(vars.public_subnet_cidrs) ?
  size(vars.private_subnet_cidrs) == size(vars.public_subnet_cidrs) :
  true
```

---

### EKS Workspace Rules

#### `eks/00_dependencies.cel`
```cel
# @target: vpc_id, subnet_ids
# @severity: error
# @workspace: eks
# @description: EKS requires VPC ID and subnet IDs from dependent workspaces
# @remediation: Ensure vpc_id and subnet_ids are passed via input mappings from vpc and subnets workspaces

has(vars.vpc_id) && vars.vpc_id != "" &&
has(vars.subnet_ids) && size(vars.subnet_ids) >= 2
```

#### `eks/01_cluster_name.cel`
```cel
# @target: cluster_name, name
# @severity: error
# @workspace: eks
# @description: EKS cluster name must follow naming conventions (lowercase, alphanumeric, hyphens)
# @remediation: Use lowercase letters, numbers, and hyphens. Max 100 chars. Example: my-prod-cluster

has(vars.cluster_name) && vars.cluster_name != "" ?
  (vars.cluster_name.matches("^[a-z][a-z0-9-]*[a-z0-9]$") &&
   size(vars.cluster_name) <= 100 &&
   size(vars.cluster_name) >= 3) :
  (has(vars.name) ? vars.name.matches("^[a-z][a-z0-9-]*[a-z0-9]$") : false)
```

#### `eks/02_version.cel`
```cel
# @target: cluster_version, kubernetes_version
# @severity: error
# @workspace: eks
# @description: Kubernetes version must be a supported EKS version
# @remediation: Use a supported version: 1.28, 1.29, 1.30, 1.31. Check AWS docs for current supported versions

has(vars.cluster_version) ?
  vars.cluster_version.matches("^1\\.(2[89]|3[01])$") :
  (has(vars.kubernetes_version) ?
    vars.kubernetes_version.matches("^1\\.(2[89]|3[01])$") : true)
```

#### `eks/03_node_sizing.cel`
```cel
# @target: node_desired_size, node_min_size, node_max_size, desired_size, min_size, max_size
# @severity: error
# @workspace: eks
# @description: Node group sizing must be valid (min <= desired <= max, min >= 1)
# @remediation: Set min_size >= 1, and ensure min_size <= desired_size <= max_size

// Check node group sizing constraints
has(vars.node_min_size) && has(vars.node_max_size) && has(vars.node_desired_size) ?
  (vars.node_min_size >= 1 &&
   vars.node_min_size <= vars.node_desired_size &&
   vars.node_desired_size <= vars.node_max_size) :
  (has(vars.min_size) && has(vars.max_size) && has(vars.desired_size) ?
    (vars.min_size >= 1 && vars.min_size <= vars.desired_size && vars.desired_size <= vars.max_size) :
    true)
```

#### `eks/04_subnet_count.cel`
```cel
# @target: subnet_ids
# @severity: error
# @workspace: eks
# @description: EKS requires at least 2 subnets in different AZs for high availability
# @remediation: Provide subnet_ids with at least 2 subnets from different availability zones

has(vars.subnet_ids) ? size(vars.subnet_ids) >= 2 : false
```

#### `eks/05_instance_types.cel`
```cel
# @target: node_instance_types, instance_types
# @severity: warning
# @workspace: eks
# @description: Node instance types should be appropriate for workloads
# @remediation: Use m5.large or larger for production. Avoid t2/t3 for production workloads

has(vars.node_instance_types) && size(vars.node_instance_types) > 0 ?
  !vars.node_instance_types.exists(t, t.startsWith("t2.") || t.startsWith("t3.micro") || t.startsWith("t3.nano")) :
  (has(vars.instance_types) && size(vars.instance_types) > 0 ?
    !vars.instance_types.exists(t, t.startsWith("t2.") || t.startsWith("t3.micro")) : true)
```

#### `eks/06_endpoint_access.cel`
```cel
# @target: cluster_endpoint_public_access, cluster_endpoint_private_access
# @severity: warning
# @workspace: eks
# @description: Consider restricting public endpoint access for production clusters
# @remediation: Set cluster_endpoint_public_access=false and cluster_endpoint_private_access=true for secure clusters

has(vars.cluster_endpoint_public_access) && has(vars.cluster_endpoint_private_access) ?
  (vars.cluster_endpoint_private_access == true) :
  true  // Pass if not specified
```

#### `eks/07_encryption.cel`
```cel
# @target: cluster_encryption_config, enable_cluster_encryption
# @severity: warning
# @workspace: eks
# @description: EKS secrets encryption should be enabled for production
# @remediation: Enable cluster encryption with a KMS key for secrets at rest

has(vars.enable_cluster_encryption) ?
  vars.enable_cluster_encryption == true :
  (has(vars.cluster_encryption_config) ? size(vars.cluster_encryption_config) > 0 : true)
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
| `isRFC1918` | `(string) -> bool` | Validates RFC 1918 private IP range (10.x, 172.16-31.x, 192.168.x) |
| `isCIDR` | `(string) -> bool` | Validates CIDR notation format (e.g., 10.0.0.0/16) |
| `isValidIP` | `(string) -> bool` | Validates IPv4 address format |
| `cidrContains` | `(string, string) -> bool` | Checks if first CIDR contains the second IP/CIDR |
| `cidrsOverlap` | `(list, list) -> bool` | Checks if any CIDRs in two lists overlap |
| `cidrSize` | `(string) -> int` | Returns the prefix length of a CIDR (e.g., 16 for /16) |
| `cidrHostCount` | `(string) -> int` | Returns the number of usable hosts in a CIDR |

### CEL Function Implementations

```go
// In validation/validators.go

// isRFC1918 checks if a CIDR or IP is within RFC 1918 private address space
func isRFC1918(addr string) bool {
    ip := extractIP(addr)
    if ip == nil {
        return false
    }

    // RFC 1918 ranges
    private10 := net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)}
    private172 := net.IPNet{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)}
    private192 := net.IPNet{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)}

    return private10.Contains(ip) || private172.Contains(ip) || private192.Contains(ip)
}

// isCIDR validates CIDR notation
func isCIDR(cidr string) bool {
    _, _, err := net.ParseCIDR(cidr)
    return err == nil
}

// cidrsOverlap checks if any CIDRs in the two slices overlap
func cidrsOverlap(cidrs1, cidrs2 []string) bool {
    for _, c1 := range cidrs1 {
        _, net1, err1 := net.ParseCIDR(c1)
        if err1 != nil {
            continue
        }
        for _, c2 := range cidrs2 {
            _, net2, err2 := net.ParseCIDR(c2)
            if err2 != nil {
                continue
            }
            if net1.Contains(net2.IP) || net2.Contains(net1.IP) {
                return true
            }
        }
    }
    return false
}
```

---

## Resolved Design Decisions

| Decision | Resolution |
|----------|------------|
| **Rule Precedence** | Last rule wins. Rules evaluated: common → workspace-specific → alphabetical by filename → @order metadata |
| **Workspace-Specific Rules** | Yes, rules filtered by `@workspace` metadata. Directory structure maps to workspace names (vpc/, subnets/, eks/) |

## Open Questions

1. **Runtime vs Static**: Should some validations run after `terraform plan` output (e.g., cost estimation)?
2. **Rule Versioning**: How to handle rule updates across deployments? Semantic versioning for rule sets?
3. **Performance**: Should compiled CEL programs be cached between invocations? (Recommended: yes, at service startup)
4. **Rule Inheritance**: Should workspace rules be able to explicitly disable common rules?
5. **Cross-Workspace Validation**: Should rules validate consistency across workspaces (e.g., VPC CIDR contains subnet CIDRs)?

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
