package enrich

import (
	"strings"
	"testing"

	"github.com/c4a8-azure/azlift/internal/refine"
)

func TestGenerateCommonTagsOutput(t *testing.T) {
	pf := refine.NewFile("/tmp/outputs.tf")
	GenerateCommonTagsOutput(pf)

	out := string(pf.File.Bytes())
	if !strings.Contains(out, "common_tags") {
		t.Error("expected common_tags output block")
	}
	if !strings.Contains(out, "local.common_tags") {
		t.Error("expected output to reference local.common_tags")
	}
}
