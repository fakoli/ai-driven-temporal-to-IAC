package validation

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELEngine manages the CEL environment and program execution
type CELEngine struct {
	env      *cel.Env
	programs map[string]cel.Program // Cached compiled programs by rule ID
}

// NewCELEngine creates a new CEL engine with custom functions
func NewCELEngine() (*CELEngine, error) {
	env, err := createCELEnvironment()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &CELEngine{
		env:      env,
		programs: make(map[string]cel.Program),
	}, nil
}

// createCELEnvironment sets up the CEL environment with custom functions and variables
func createCELEnvironment() (*cel.Env, error) {
	return cel.NewEnv(
		// Declare variables available to rules
		cel.Variable("vars", cel.MapType(cel.StringType, cel.DynType)),
		cel.Variable("workspace", cel.MapType(cel.StringType, cel.StringType)),

		// Custom functions for validation
		cel.Function("isRFC1918",
			cel.Overload("isRFC1918_string",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(isRFC1918Binding),
			),
		),

		cel.Function("isCIDR",
			cel.Overload("isCIDR_string",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(isCIDRBinding),
			),
		),

		cel.Function("isValidIP",
			cel.Overload("isValidIP_string",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(isValidIPBinding),
			),
		),

		cel.Function("cidrContains",
			cel.Overload("cidrContains_string_string",
				[]*cel.Type{cel.StringType, cel.StringType},
				cel.BoolType,
				cel.BinaryBinding(cidrContainsBinding),
			),
		),

		cel.Function("cidrsOverlap",
			cel.Overload("cidrsOverlap_list_list",
				[]*cel.Type{cel.ListType(cel.StringType), cel.ListType(cel.StringType)},
				cel.BoolType,
				cel.BinaryBinding(cidrsOverlapBinding),
			),
		),

		cel.Function("cidrSize",
			cel.Overload("cidrSize_string",
				[]*cel.Type{cel.StringType},
				cel.IntType,
				cel.UnaryBinding(cidrSizeBinding),
			),
		),

		cel.Function("cidrHostCount",
			cel.Overload("cidrHostCount_string",
				[]*cel.Type{cel.StringType},
				cel.IntType,
				cel.UnaryBinding(cidrHostCountBinding),
			),
		),

		cel.Function("isValidAWSRegion",
			cel.Overload("isValidAWSRegion_string",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(isValidAWSRegionBinding),
			),
		),

		cel.Function("isValidEKSVersion",
			cel.Overload("isValidEKSVersion_string",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(isValidEKSVersionBinding),
			),
		),

		cel.Function("isValidClusterName",
			cel.Overload("isValidClusterName_string",
				[]*cel.Type{cel.StringType},
				cel.BoolType,
				cel.UnaryBinding(isValidClusterNameBinding),
			),
		),

		cel.Function("allRFC1918",
			cel.Overload("allRFC1918_list",
				[]*cel.Type{cel.ListType(cel.StringType)},
				cel.BoolType,
				cel.UnaryBinding(allRFC1918Binding),
			),
		),
	)
}

// CompileRule compiles a CEL expression and caches it
func (e *CELEngine) CompileRule(rule *Rule) (cel.Program, error) {
	// Check cache first
	if prog, ok := e.programs[rule.ID]; ok {
		return prog, nil
	}

	// Parse the expression
	ast, issues := e.env.Compile(rule.Expression)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("failed to compile rule %s: %w", rule.ID, issues.Err())
	}

	// Create program
	prog, err := e.env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("failed to create program for rule %s: %w", rule.ID, err)
	}

	// Cache the program
	e.programs[rule.ID] = prog

	return prog, nil
}

// EvaluateRule evaluates a compiled rule against the given variables
func (e *CELEngine) EvaluateRule(rule *Rule, tfvars map[string]interface{}, wsCtx WorkspaceContext) (bool, error) {
	prog, err := e.CompileRule(rule)
	if err != nil {
		return false, err
	}

	// Create activation with variables
	activation := map[string]interface{}{
		"vars": tfvars,
		"workspace": map[string]string{
			"name": wsCtx.Name,
			"kind": wsCtx.Kind,
			"dir":  wsCtx.Dir,
		},
	}

	// Evaluate
	out, _, err := prog.Eval(activation)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate rule %s: %w", rule.ID, err)
	}

	// Convert result to bool
	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("rule %s did not return boolean value, got %T", rule.ID, out.Value())
	}

	return result, nil
}

// GetEnvironment returns the CEL environment
func (e *CELEngine) GetEnvironment() *cel.Env {
	return e.env
}

// CEL function bindings

func isRFC1918Binding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	return types.Bool(IsRFC1918(str))
}

func isCIDRBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	return types.Bool(IsCIDR(str))
}

func isValidIPBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	return types.Bool(IsValidIP(str))
}

func cidrContainsBinding(arg1, arg2 ref.Val) ref.Val {
	container, ok1 := arg1.Value().(string)
	contained, ok2 := arg2.Value().(string)
	if !ok1 || !ok2 {
		return types.Bool(false)
	}
	return types.Bool(CIDRContains(container, contained))
}

func cidrsOverlapBinding(arg1, arg2 ref.Val) ref.Val {
	cidrs1 := refValToStringSlice(arg1)
	cidrs2 := refValToStringSlice(arg2)
	return types.Bool(CIDRsOverlap(cidrs1, cidrs2))
}

func cidrSizeBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Int(-1)
	}
	return types.Int(CIDRSize(str))
}

func cidrHostCountBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Int(0)
	}
	return types.Int(CIDRHostCount(str))
}

func isValidAWSRegionBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	return types.Bool(IsValidAWSRegion(str))
}

func isValidEKSVersionBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	return types.Bool(IsValidEKSVersion(str))
}

func isValidClusterNameBinding(arg ref.Val) ref.Val {
	str, ok := arg.Value().(string)
	if !ok {
		return types.Bool(false)
	}
	return types.Bool(IsValidClusterName(str))
}

func allRFC1918Binding(arg ref.Val) ref.Val {
	cidrs := refValToStringSlice(arg)
	return types.Bool(AllCIDRsRFC1918(cidrs))
}

// refValToStringSlice converts a CEL list to a Go string slice
func refValToStringSlice(val ref.Val) []string {
	var result []string

	// Handle the value based on its underlying type
	switch v := val.Value().(type) {
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	case []string:
		result = v
	case []ref.Val:
		for _, item := range v {
			if str, ok := item.Value().(string); ok {
				result = append(result, str)
			}
		}
	default:
		// Try reflection as fallback
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice {
			for i := 0; i < rv.Len(); i++ {
				elem := rv.Index(i).Interface()
				if str, ok := elem.(string); ok {
					result = append(result, str)
				} else if refVal, ok := elem.(ref.Val); ok {
					if str, ok := refVal.Value().(string); ok {
						result = append(result, str)
					}
				}
			}
		}
	}

	return result
}
