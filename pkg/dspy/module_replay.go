package dspy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModuleReplaySpan is a portable Process-span fixture for offline and live replay.
// Hosts typically extract these from AttachModuleTrace JSONL dumps.
type ModuleReplaySpan struct {
	Source           string         `json:"source,omitempty"`
	Operation        string         `json:"operation,omitempty"`
	Task             string         `json:"task,omitempty"`
	Fields           map[string]any `json:"fields"`
	RecordedOutputs  map[string]any `json:"recorded_outputs"`
}

// LoadModuleReplaySpan reads a span JSON file (fields + recorded_outputs).
func LoadModuleReplaySpan(path string) (*ModuleReplaySpan, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("LoadModuleReplaySpan: path is required")
	}
	// Path is caller-supplied fixture path (tests / host capture).
	data, err := os.ReadFile(path) //nolint:gosec // G304: intentional fixture load
	if err != nil {
		return nil, fmt.Errorf("LoadModuleReplaySpan: read %s: %w", path, err)
	}
	var span ModuleReplaySpan
	if err := json.Unmarshal(data, &span); err != nil {
		return nil, fmt.Errorf("LoadModuleReplaySpan: unmarshal %s: %w", path, err)
	}
	if span.Fields == nil {
		return nil, fmt.Errorf("LoadModuleReplaySpan: %s missing fields", path)
	}
	if span.RecordedOutputs == nil {
		return nil, fmt.Errorf("LoadModuleReplaySpan: %s missing recorded_outputs", path)
	}
	return &span, nil
}

// ModuleReplayFixturePath returns testdata/module-replay/<name> relative to this package.
func ModuleReplayFixturePath(name string) string {
	return filepath.Join("testdata", "module-replay", name)
}
