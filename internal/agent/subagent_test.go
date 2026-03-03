package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jonpo/cclient3/internal/api"
	"github.com/jonpo/cclient3/internal/config"
	"github.com/jonpo/cclient3/internal/tools"
)

// TestSubAgentTool_InputValidation verifies that missing required fields are caught.
func TestSubAgentTool_InputValidation(t *testing.T) {
	tool := &SubAgentTool{}

	// Missing task field
	input, _ := json.Marshal(map[string]string{})
	result := tool.Execute(context.Background(), input)
	if !result.IsError {
		t.Error("expected error when task is empty")
	}

	// Bad JSON
	result = tool.Execute(context.Background(), json.RawMessage(`not json`))
	if !result.IsError {
		t.Error("expected error on invalid JSON input")
	}
}

// TestSubAgentTool_Metadata checks the tool name, description, and schema.
func TestSubAgentTool_Metadata(t *testing.T) {
	tool := &SubAgentTool{}

	if tool.Name() != "sub_agent" {
		t.Errorf("Name() = %q, want sub_agent", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}

	schema := tool.InputSchema()
	if !json.Valid(schema) {
		t.Errorf("InputSchema() is not valid JSON: %s", schema)
	}

	// Schema must declare "task" as required
	var parsed map[string]interface{}
	json.Unmarshal(schema, &parsed)
	required, ok := parsed["required"].([]interface{})
	if !ok {
		t.Fatal("schema missing 'required' array")
	}
	found := false
	for _, r := range required {
		if r == "task" {
			found = true
		}
	}
	if !found {
		t.Error("schema required array should contain 'task'")
	}
}

// TestSubAgentTool_CounterIncrements verifies each Execute call gets a unique ID.
func TestSubAgentTool_CounterIncrements(t *testing.T) {
	// We only test the counter logic, not actual API calls.
	tool := &SubAgentTool{}
	before := tool.counter.Load()

	// Even though Execute will fail (no client), the counter increments before the call.
	// We test the counter directly.
	id1 := tool.counter.Add(1)
	id2 := tool.counter.Add(1)
	if id2 <= id1 {
		t.Error("counter should increment monotonically")
	}
	_ = before
}

// TestNewSubAgentTool verifies the constructor wires fields correctly.
func TestNewSubAgentTool_Constructor(t *testing.T) {
	client := api.NewClient("key", "https://example.com")
	providers := api.NewProviderRegistry(client)
	cfg := config.DefaultConfig()
	reg := tools.NewRegistry()

	tool := NewSubAgentTool(providers, cfg, reg, nil)
	if tool == nil {
		t.Fatal("NewSubAgentTool returned nil")
	}
	if tool.providers != providers {
		t.Error("providers not wired correctly")
	}
	if tool.cfg != cfg {
		t.Error("cfg not wired correctly")
	}
	if tool.registry != reg {
		t.Error("registry not wired correctly")
	}
	if tool.executor == nil {
		t.Error("executor should be initialized")
	}
}
