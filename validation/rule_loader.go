package validation

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Default rules path relative to project root
const DefaultRulesPath = "validation/rules"

// RuleLoader handles discovering and loading CEL rule files
type RuleLoader struct {
	rulesPath string
}

// NewRuleLoader creates a new rule loader
func NewRuleLoader(rulesPath string) *RuleLoader {
	if rulesPath == "" {
		rulesPath = DefaultRulesPath
	}
	return &RuleLoader{rulesPath: rulesPath}
}

// LoadRules loads all rules from the rules directory
func (l *RuleLoader) LoadRules() (*RuleSet, error) {
	ruleSet := &RuleSet{
		Rules:     []Rule{},
		RulesPath: l.rulesPath,
		LoadedAt:  time.Now(),
		RulesByID: make(map[string]*Rule),
		RulesByWS: make(map[string][]*Rule),
	}

	// Check if rules path exists
	if _, err := os.Stat(l.rulesPath); os.IsNotExist(err) {
		return ruleSet, nil // Return empty ruleset if no rules directory
	}

	// Walk the rules directory
	err := filepath.Walk(l.rulesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-.cel files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".cel") {
			return nil
		}

		// Load the rule
		rule, err := l.loadRuleFile(path)
		if err != nil {
			return fmt.Errorf("failed to load rule %s: %w", path, err)
		}

		ruleSet.Rules = append(ruleSet.Rules, *rule)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk rules directory: %w", err)
	}

	// Sort rules by precedence (common first, then workspace-specific, then by filename)
	sort.Slice(ruleSet.Rules, func(i, j int) bool {
		ri, rj := &ruleSet.Rules[i], &ruleSet.Rules[j]

		// Common rules come first (lowest precedence)
		iCommon := ri.Category == "common" || ri.Workspace == "*"
		jCommon := rj.Category == "common" || rj.Workspace == "*"
		if iCommon != jCommon {
			return iCommon // common rules sort first
		}

		// Then by category
		if ri.Category != rj.Category {
			return ri.Category < rj.Category
		}

		// Then by explicit order if set
		if ri.Order != rj.Order {
			return ri.Order < rj.Order
		}

		// Finally by filename (alphabetical)
		return ri.Name < rj.Name
	})

	// Build indexes
	for i := range ruleSet.Rules {
		rule := &ruleSet.Rules[i]
		ruleSet.RulesByID[rule.ID] = rule

		// Index by workspace
		wsKey := rule.Workspace
		if wsKey == "" {
			wsKey = rule.Category
		}
		ruleSet.RulesByWS[wsKey] = append(ruleSet.RulesByWS[wsKey], rule)
	}

	return ruleSet, nil
}

// loadRuleFile loads a single rule file
func (l *RuleLoader) loadRuleFile(path string) (*Rule, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	rule := &Rule{
		FilePath: path,
		Severity: SeverityError, // Default severity
	}

	// Determine category from directory name
	relPath, err := filepath.Rel(l.rulesPath, path)
	if err != nil {
		relPath = path
	}
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) > 1 {
		rule.Category = parts[0]
	}

	// Determine name from filename
	rule.Name = strings.TrimSuffix(filepath.Base(path), ".cel")
	// Remove numeric prefix if present (e.g., "00_required_region" -> "required_region")
	if match := regexp.MustCompile(`^\d+_(.+)$`).FindStringSubmatch(rule.Name); match != nil {
		rule.Name = match[1]
	}

	// Generate ID
	rule.ID = fmt.Sprintf("%s.%s", rule.Category, rule.Name)

	// Parse the file
	scanner := bufio.NewScanner(file)
	var expressionLines []string
	inExpression := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Parse metadata comments
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			// Remove comment prefix
			comment := strings.TrimPrefix(trimmed, "#")
			comment = strings.TrimPrefix(comment, "//")
			comment = strings.TrimSpace(comment)

			// Parse metadata tags
			if strings.HasPrefix(comment, "@") {
				l.parseMetadata(rule, comment)
			}
			continue
		}

		// Empty lines before expression are skipped
		if trimmed == "" && !inExpression {
			continue
		}

		// Everything else is part of the expression
		inExpression = true
		expressionLines = append(expressionLines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Join expression lines
	rule.Expression = strings.TrimSpace(strings.Join(expressionLines, "\n"))

	if rule.Expression == "" {
		return nil, fmt.Errorf("rule %s has no expression", path)
	}

	return rule, nil
}

// parseMetadata parses a metadata tag from a comment
func (l *RuleLoader) parseMetadata(rule *Rule, comment string) {
	// Remove @ prefix
	comment = strings.TrimPrefix(comment, "@")

	// Split on first colon
	parts := strings.SplitN(comment, ":", 2)
	if len(parts) != 2 {
		return
	}

	key := strings.TrimSpace(strings.ToLower(parts[0]))
	value := strings.TrimSpace(parts[1])

	switch key {
	case "target":
		// Parse comma-separated targets
		targets := strings.Split(value, ",")
		for _, t := range targets {
			t = strings.TrimSpace(t)
			if t != "" {
				rule.Target = append(rule.Target, t)
			}
		}

	case "severity":
		value = strings.ToLower(value)
		switch value {
		case "error":
			rule.Severity = SeverityError
		case "warning":
			rule.Severity = SeverityWarning
		case "info":
			rule.Severity = SeverityInfo
		}

	case "description":
		rule.Description = value

	case "remediation":
		rule.Remediation = value

	case "workspace":
		rule.Workspace = value

	case "order":
		if order, err := strconv.Atoi(value); err == nil {
			rule.Order = order
		}
	}
}

// LoadRulesFromPath is a convenience function to load rules from a path
func LoadRulesFromPath(rulesPath string) (*RuleSet, error) {
	loader := NewRuleLoader(rulesPath)
	return loader.LoadRules()
}

// GetRulesByCategory returns all rules in a specific category
func (rs *RuleSet) GetRulesByCategory(category string) []*Rule {
	var rules []*Rule
	for i := range rs.Rules {
		if rs.Rules[i].Category == category {
			rules = append(rules, &rs.Rules[i])
		}
	}
	return rules
}

// GetApplicableRules returns rules applicable to a workspace, properly ordered by precedence
func (rs *RuleSet) GetApplicableRules(workspaceName string) []*Rule {
	var rules []*Rule

	// Add rules in precedence order (already sorted in LoadRules)
	for i := range rs.Rules {
		rule := &rs.Rules[i]

		// Check if rule applies to this workspace
		if rule.Workspace == "*" || rule.Workspace == "" {
			// Common rules apply to all
			if rule.Category == "common" || rule.Category == workspaceName {
				rules = append(rules, rule)
			}
		} else if rule.Workspace == workspaceName || rule.Category == workspaceName {
			rules = append(rules, rule)
		}
	}

	return rules
}

// String returns a summary of the ruleset
func (rs *RuleSet) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("RuleSet: %d rules loaded from %s\n", len(rs.Rules), rs.RulesPath))

	categories := make(map[string]int)
	for _, rule := range rs.Rules {
		categories[rule.Category]++
	}

	for cat, count := range categories {
		b.WriteString(fmt.Sprintf("  - %s: %d rules\n", cat, count))
	}

	return b.String()
}
