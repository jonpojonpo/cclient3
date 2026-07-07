package agent

import (
	"context"
	"encoding/json"
	"strings"
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

// TestSubAgentIDs_Unique verifies IDs are unique across tool instances
// (nested sub-agent tools share one global counter so tab IDs never collide).
func TestSubAgentIDs_Unique(t *testing.T) {
	id1 := nextSubAgentID()
	id2 := nextSubAgentID()
	if id2 <= id1 {
		t.Error("sub-agent IDs should increment monotonically across instances")
	}
}

// TestNewSubAgentTool verifies the constructor wires fields correctly.
func TestNewSubAgentTool_Constructor(t *testing.T) {
	client := api.NewClient("key", "https://example.com")
	providers := api.NewProviderRegistry(client)
	cfg := config.DefaultConfig()
	reg := tools.NewRegistry()

	tool := NewSubAgentTool(providers, cfg, reg, nil, 1)
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

// TestSubAgentTool_DepthAwareDescription verifies the tool tells agents at
// the max depth that they cannot delegate further.
func TestSubAgentTool_DepthAwareDescription(t *testing.T) {
	mid := &SubAgentTool{depth: 1}
	leaf := &SubAgentTool{depth: MaxSubAgentDepth}

	if !strings.Contains(mid.Description(), "delegate one") {
		t.Error("depth-1 description should say further delegation is allowed")
	}
	if !strings.Contains(leaf.Description(), "CANNOT spawn further") {
		t.Error("max-depth description should forbid further delegation")
	}
}

// TestNewAgent_RegistryChain verifies the main agent exposes sub_agent, and
// that the chain terminates: the deepest registry has no sub_agent tool.
func TestNewAgent_RegistryChain(t *testing.T) {
	client := api.NewClient("key", "https://example.com")
	providers := api.NewProviderRegistry(client)
	cfg := config.DefaultConfig()

	ag := NewAgent(cfg, providers, nil)
	defer ag.Shutdown()

	// Walk the chain: main registry → depth-1 children → depth-2 children.
	reg := ag.registry
	for depth := 1; depth <= MaxSubAgentDepth; depth++ {
		toolIface, err := reg.Get("sub_agent")
		if err != nil {
			t.Fatalf("registry at depth %d should expose sub_agent: %v", depth-1, err)
		}
		sat, ok := toolIface.(*SubAgentTool)
		if !ok {
			t.Fatalf("sub_agent has unexpected type %T", toolIface)
		}
		if sat.depth != depth {
			t.Errorf("sub_agent spawns depth %d, want %d", sat.depth, depth)
		}
		reg = sat.registry
	}
	if _, err := reg.Get("sub_agent"); err == nil {
		t.Errorf("leaf registry (depth %d) must not expose sub_agent", MaxSubAgentDepth)
	}
}
