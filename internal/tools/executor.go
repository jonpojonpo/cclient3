package tools

import (
	"context"
	"fmt"
	"sync"
)

// Executor runs tool calls in parallel with bounded concurrency.
type Executor struct {
	registry   *Registry
	maxWorkers int
}

func NewExecutor(registry *Registry, maxWorkers int) *Executor {
	if maxWorkers < 1 {
		maxWorkers = 6
	}
	return &Executor{
		registry:   registry,
		maxWorkers: maxWorkers,
	}
}

// ExecuteAll runs all tool calls concurrently (up to maxWorkers) and returns results in order.
func (e *Executor) ExecuteAll(ctx context.Context, calls []ToolCall) []ToolCallResult {
	results := make([]ToolCallResult, len(calls))
	sem := make(chan struct{}, e.maxWorkers)
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc ToolCall) {
			defer wg.Done()

			// Respect cancellation before acquiring semaphore slot.
			select {
			case <-ctx.Done():
				results[idx] = ToolCallResult{
					Call:   tc,
					Result: ToolResult{Error: ctx.Err().Error(), IsError: true},
				}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			tool, err := e.registry.Get(tc.Name)
			if err != nil {
				results[idx] = ToolCallResult{
					Call: tc,
					Result: ToolResult{
						Error:   fmt.Sprintf("Tool not found: %s", tc.Name),
						IsError: true,
					},
				}
				return
			}

			result := tool.Execute(ctx, tc.Input)
			results[idx] = ToolCallResult{
				Call:   tc,
				Result: result,
			}
		}(i, call)
	}

	wg.Wait()
	return results
}
