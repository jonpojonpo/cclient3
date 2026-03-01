package tools

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"
)

// mockTool is a tool that sleeps then returns a fixed output.
type mockTool struct {
	name   string
	delay  time.Duration
	output string
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string  { return "mock" }
func (t *mockTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *mockTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	time.Sleep(t.delay)
	return ToolResult{Output: t.output}
}

func TestExecutor_Parallel(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{name: "slow_a", delay: 100 * time.Millisecond, output: "a"})
	reg.Register(&mockTool{name: "slow_b", delay: 100 * time.Millisecond, output: "b"})
	reg.Register(&mockTool{name: "slow_c", delay: 100 * time.Millisecond, output: "c"})

	exec := NewExecutor(reg, 6)

	calls := []ToolCall{
		{ID: "1", Name: "slow_a", Input: json.RawMessage(`{}`)},
		{ID: "2", Name: "slow_b", Input: json.RawMessage(`{}`)},
		{ID: "3", Name: "slow_c", Input: json.RawMessage(`{}`)},
	}

	start := time.Now()
	results := exec.ExecuteAll(context.Background(), calls)
	elapsed := time.Since(start)

	// All three should run in parallel, so total time ~ 100ms, not 300ms
	if elapsed > 250*time.Millisecond {
		t.Errorf("expected parallel execution (<250ms), took %v", elapsed)
	}

	// Verify ordering
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Result.Output != "a" {
		t.Errorf("result[0]: got %q, want 'a'", results[0].Result.Output)
	}
	if results[1].Result.Output != "b" {
		t.Errorf("result[1]: got %q, want 'b'", results[1].Result.Output)
	}
	if results[2].Result.Output != "c" {
		t.Errorf("result[2]: got %q, want 'c'", results[2].Result.Output)
	}
}

func TestExecutor_Semaphore(t *testing.T) {
	var concurrent int64
	var maxConcurrent int64

	reg := NewRegistry()

	// A tool that tracks concurrency
	concTool := &concurrencyTool{
		concurrent:    &concurrent,
		maxConcurrent: &maxConcurrent,
	}
	reg.Register(concTool)

	// Semaphore of 2 with 5 tool calls
	exec := NewExecutor(reg, 2)

	calls := make([]ToolCall, 5)
	for i := range calls {
		calls[i] = ToolCall{ID: string(rune('a' + i)), Name: "conc_tool", Input: json.RawMessage(`{}`)}
	}

	exec.ExecuteAll(context.Background(), calls)

	if maxConcurrent > 2 {
		t.Errorf("max concurrent %d exceeded semaphore limit 2", maxConcurrent)
	}
}

func TestExecutor_UnknownTool(t *testing.T) {
	reg := NewRegistry()
	exec := NewExecutor(reg, 6)

	calls := []ToolCall{
		{ID: "1", Name: "nonexistent", Input: json.RawMessage(`{}`)},
	}

	results := exec.ExecuteAll(context.Background(), calls)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Result.IsError {
		t.Error("expected error for unknown tool")
	}
}

type concurrencyTool struct {
	concurrent    *int64
	maxConcurrent *int64
}

func (t *concurrencyTool) Name() string        { return "conc_tool" }
func (t *concurrencyTool) Description() string  { return "concurrency test" }
func (t *concurrencyTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (t *concurrencyTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	curr := atomic.AddInt64(t.concurrent, 1)
	for {
		max := atomic.LoadInt64(t.maxConcurrent)
		if curr > max {
			if atomic.CompareAndSwapInt64(t.maxConcurrent, max, curr) {
				break
			}
		} else {
			break
		}
	}
	time.Sleep(50 * time.Millisecond)
	atomic.AddInt64(t.concurrent, -1)
	return ToolResult{Output: "ok"}
}
