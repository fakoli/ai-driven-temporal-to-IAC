# Validation Rules

This directory contains CEL (Common Expression Language) rules for validating Terraform variable files before execution.

## Directory Structure

```
rules/
├── common/          # Rules that apply to all workspaces
├── vpc/             # VPC-specific validation rules
├── subnets/         # Subnet-specific validation rules
├── eks/             # EKS-specific validation rules
└── README.md        # This file
```

## Rule File Format

Each rule is a `.cel` file with metadata in comments:

```cel
# @target: variable_name, another_variable
# @severity: error
# @workspace: vpc
# @description: Human-readable description
# @remediation: How to fix the issue

has(vars.variable_name) ? isRFC1918(vars.variable_name) : true
```

## Metadata Fields

| Field | Required | Description |
|-------|----------|-------------|
| `@target` | Yes | Comma-separated list of variables this rule validates |
| `@severity` | Yes | `error`, `warning`, or `info` |
| `@description` | Yes | Human-readable description shown on failure |
| `@remediation` | No | Suggested fix for the issue |
| `@workspace` | No | Workspace name pattern (`*` for all, or specific name like `vpc`) |
| `@order` | No | Explicit evaluation order (default: filename order) |

## Available CEL Functions

### Network Validation

| Function | Signature | Description |
|----------|-----------|-------------|
| `isRFC1918(addr)` | `string -> bool` | Check if IP/CIDR is RFC 1918 private |
| `isCIDR(cidr)` | `string -> bool` | Validate CIDR notation |
| `isValidIP(ip)` | `string -> bool` | Validate IPv4 address |
| `cidrContains(container, contained)` | `(string, string) -> bool` | Check CIDR containment |
| `cidrsOverlap(list1, list2)` | `(list, list) -> bool` | Check for CIDR overlap |
| `cidrSize(cidr)` | `string -> int` | Get CIDR prefix length |
| `cidrHostCount(cidr)` | `string -> int` | Get usable host count |
| `allRFC1918(cidrs)` | `list -> bool` | Check all CIDRs are RFC 1918 |

### AWS Validation

| Function | Signature | Description |
|----------|-----------|-------------|
| `isValidAWSRegion(region)` | `string -> bool` | Validate AWS region format |
| `isValidEKSVersion(version)` | `string -> bool` | Check EKS K8s version support |
| `isValidClusterName(name)` | `string -> bool` | Validate EKS cluster naming |

## Rule Precedence

Rules are evaluated in order:

1. `common/*.cel` - Lowest precedence (base rules)
2. `{workspace}/*.cel` - Workspace-specific rules (override common)
3. Within directory - Alphabetical by filename (`00_` before `01_`)
4. `@order` metadata - Explicit override if specified

## Context Variables

Rules have access to:

- `vars` - The combined tfvars map
- `workspace.name` - Current workspace name
- `workspace.kind` - Workspace kind (terraform, tofu)
- `workspace.dir` - Workspace directory path

## Examples

### Required Field
```cel
# @target: region
# @severity: error
# @description: Region is required
has(vars.region) && vars.region != ""
```

### RFC 1918 Validation
```cel
# @target: vpc_cidr
# @severity: error
# @description: VPC must use private address space
has(vars.vpc_cidr) ? isRFC1918(vars.vpc_cidr) : true
```

### List Validation
```cel
# @target: subnet_ids
# @severity: error
# @description: At least 2 subnets required
has(vars.subnet_ids) ? size(vars.subnet_ids) >= 2 : false
```

### Conditional Validation
```cel
# @target: cluster_version
# @severity: warning
# @workspace: eks
# @description: EKS version should be current
has(vars.cluster_version) ?
  isValidEKSVersion(vars.cluster_version) :
  true
```

## Writing New Rules

1. Create a new `.cel` file in the appropriate directory
2. Add required metadata comments (`@target`, `@severity`, `@description`)
3. Write the CEL expression (must return boolean)
4. Test with the validator CLI: `go run ./cmd/validator -config infra.yaml`

## Testing Rules

```bash
# Validate all workspaces
go run ./cmd/validator -config infra.yaml

# Validate specific workspace
go run ./cmd/validator -config infra.yaml -workspace vpc

# Show JSON output
go run ./cmd/validator -config infra.yaml -format json
```
