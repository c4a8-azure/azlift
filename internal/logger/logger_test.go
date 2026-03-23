package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestTextOutput_ContainsStagePrefix(t *testing.T) {
	var buf bytes.Buffer
	log := New(StageScan, Options{Writer: &buf})
	log.Info("hello world")

	out := buf.String()
	if !strings.Contains(out, "[SCAN") {
		t.Errorf("expected [SCAN prefix in output, got: %s", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected message in output, got: %s", out)
	}
}

func TestTextOutput_AllStages(t *testing.T) {
	stages := []Stage{StageScan, StageExport, StageRefine, StageBootstrap, StageRun}
	for _, stage := range stages {
		var buf bytes.Buffer
		log := New(stage, Options{Writer: &buf})
		log.Info("test")
		if !strings.Contains(buf.String(), string(stage)) {
			t.Errorf("stage %s not found in output: %s", stage, buf.String())
		}
	}
}

func TestVerbose_DebugVisible(t *testing.T) {
	var buf bytes.Buffer
	log := New(StageRoot, Options{Verbose: true, Writer: &buf})
	log.Debug("debug message")
	if !strings.Contains(buf.String(), "debug message") {
		t.Errorf("debug message should be visible in verbose mode, got: %s", buf.String())
	}
}

func TestNotVerbose_DebugHidden(t *testing.T) {
	var buf bytes.Buffer
	log := New(StageRoot, Options{Verbose: false, Writer: &buf})
	log.Debug("hidden debug")
	if strings.Contains(buf.String(), "hidden debug") {
		t.Errorf("debug message should be hidden in non-verbose mode, got: %s", buf.String())
	}
}

func TestJSONFormat_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	log := New(StageScan, Options{Format: FormatJSON, Writer: &buf})
	log.Info("json test", "key", "value")

	line := strings.TrimSpace(buf.String())
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, line)
	}
	if record["msg"] != "json test" {
		t.Errorf("msg field: want 'json test', got %v", record["msg"])
	}
}

func TestWithStage_ChangesPrefix(t *testing.T) {
	var buf bytes.Buffer
	root := New(StageRoot, Options{Writer: &buf})
	scan := root.WithStage(StageScan)
	scan.Info("stage switched")

	if !strings.Contains(buf.String(), "SCAN") {
		t.Errorf("expected SCAN in output after WithStage, got: %s", buf.String())
	}
}

func TestWith_AttrsAppended(t *testing.T) {
	var buf bytes.Buffer
	log := New(StageRefine, Options{Writer: &buf}).With("file", "main.tf")
	log.Info("processing")

	if !strings.Contains(buf.String(), "main.tf") {
		t.Errorf("expected attr value in output, got: %s", buf.String())
	}
}

func TestEnabled(t *testing.T) {
	var buf bytes.Buffer
	log := New(StageRoot, Options{Verbose: false, Writer: &buf})
	if log.Enabled(slog.LevelDebug) {
		t.Error("debug should not be enabled when Verbose=false")
	}
	verbose := New(StageRoot, Options{Verbose: true, Writer: &buf})
	if !verbose.Enabled(slog.LevelDebug) {
		t.Error("debug should be enabled when Verbose=true")
	}
}
