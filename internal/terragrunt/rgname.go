package terragrunt

import (
	"sort"
	"strings"
)

// sortedKeys returns the keys of m sorted alphabetically.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// deriveEnvRGInputs produces a map of variable name → RG name for targetEnv
// by substituting the primaryEnv label with targetEnv in each RG value.
//
// Example: primaryEnv="prod", targetEnv="dev"
//
//	{"resource_group_name": "rg-myapp-prod"} → {"resource_group_name": "rg-myapp-dev"}
//
// When the primaryEnv label is not found in the value and sourceRG is known,
// the same substitution is attempted on sourceRG. Otherwise "-<targetEnv>" is
// appended.
func deriveEnvRGInputs(rgLocals map[string]string, _ string, primaryEnv, targetEnv string) map[string]string {
	out := make(map[string]string, len(rgLocals))
	for name, val := range rgLocals {
		out[name] = substituteEnv(val, primaryEnv, targetEnv)
	}
	return out
}

// substituteEnv replaces the primary environment label in val with targetEnv.
// Tries a "-<fromEnv>" suffix replacement first (most common Azure naming convention),
// then a general substring replacement, and finally appends "-<targetEnv>".
func substituteEnv(val, fromEnv, toEnv string) string {
	if fromEnv == toEnv {
		return val
	}
	// Prefer replacing "-<fromEnv>" suffix (rg-myapp-prod → rg-myapp-dev).
	suffix := "-" + fromEnv
	if strings.HasSuffix(val, suffix) {
		return val[:len(val)-len(suffix)] + "-" + toEnv
	}
	// Fall back to replacing the first occurrence of fromEnv anywhere.
	if strings.Contains(val, fromEnv) {
		return strings.Replace(val, fromEnv, toEnv, 1)
	}
	// No match: append "-<toEnv>".
	return val + "-" + toEnv
}
