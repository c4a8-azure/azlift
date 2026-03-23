package tools

import (
	"fmt"
	"regexp"
	"testing"
)

// --- extractVersion ---

func TestExtractVersion(t *testing.T) {
	cases := []struct {
		output string
		re     *regexp.Regexp
		want   string
	}{
		{"azure-cli 2.57.0", regexp.MustCompile(`(\d+\.\d+\.\d+)`), "2.57.0"},
		{"gh version 2.40.1 (2024-01-15)", regexp.MustCompile(`gh version (\d+\.\d+\.\d+)`), "2.40.1"},
		{"v0.14.3", regexp.MustCompile(`v?(\d+\.\d+\.\d+)`), "0.14.3"},
		{"no version here", regexp.MustCompile(`(\d+\.\d+\.\d+)`), ""},
	}
	for _, c := range cases {
		got := extractVersion(c.output, c.re)
		if got != c.want {
			t.Errorf("extractVersion(%q) = %q, want %q", c.output, got, c.want)
		}
	}
}

// --- checkMinVersion ---

func TestCheckMinVersion_Passing(t *testing.T) {
	cases := [][2]string{
		{"2.57.0", "2.50.0"}, // newer minor
		{"3.0.0", "2.50.0"},  // newer major
		{"2.50.0", "2.50.0"}, // exactly equal
		{"0.15.0", "0.14.0"}, // newer minor, zero major
		{"1.6.3", "1.5.0"},   // newer minor
	}
	for _, c := range cases {
		if err := checkMinVersion("tool", c[0], parseMin(c[1]), "url"); err != nil {
			t.Errorf("version %s >= min %s: unexpected error: %v", c[0], c[1], err)
		}
	}
}

func TestCheckMinVersion_Failing(t *testing.T) {
	cases := [][2]string{
		{"2.49.9", "2.50.0"}, // older minor
		{"1.9.0", "2.0.0"},   // older major
		{"0.13.5", "0.14.0"}, // older minor, zero major
	}
	for _, c := range cases {
		if err := checkMinVersion("tool", c[0], parseMin(c[1]), "url"); err == nil {
			t.Errorf("version %s < min %s: expected error, got nil", c[0], c[1])
		}
	}
}

// --- CheckResult.String ---

func TestCheckResult_String_OK(t *testing.T) {
	r := CheckResult{Tool: &Tool{Name: "az"}, Found: true, Version: "2.57.0"}
	s := r.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
	if r.Err != nil {
		t.Error("Err should be nil for passing result")
	}
}

func TestCheckResult_String_Fail(t *testing.T) {
	r := CheckResult{
		Tool:  &Tool{Name: "az"},
		Found: false,
		Err:   fmt.Errorf("az: not found"),
	}
	s := r.String()
	if s == "" {
		t.Error("String() should not be empty on failure")
	}
}

// parseMin parses "major.minor.patch" into [3]int — test helper only.
func parseMin(v string) [3]int {
	var a, b, c int
	_, _ = fmt.Sscanf(v, "%d.%d.%d", &a, &b, &c)
	return [3]int{a, b, c}
}
