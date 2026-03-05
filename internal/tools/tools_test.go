package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonpo/cclient3/internal/tools"
)

// ── BashTool ─────────────────────────────────────────────────────────────────

func TestBashTool_Echo(t *testing.T) {
	tool := tools.NewBashTool(10, nil)
	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo hello_world",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello_world") {
		t.Errorf("expected output to contain 'hello_world', got: %q", result.Output)
	}
}

func TestBashTool_MultilineOutput(t *testing.T) {
	tool := tools.NewBashTool(10, nil)
	input, _ := json.Marshal(map[string]interface{}{
		"command": "printf 'line1\nline2\nline3\n'",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	for _, line := range []string{"line1", "line2", "line3"} {
		if !strings.Contains(result.Output, line) {
			t.Errorf("expected %q in output, got: %q", line, result.Output)
		}
	}
}

func TestBashTool_ExitError(t *testing.T) {
	tool := tools.NewBashTool(10, nil)
	input, _ := json.Marshal(map[string]interface{}{
		"command": "exit 1",
	})
	result := tool.Execute(context.Background(), input)
	if !result.IsError {
		t.Error("expected IsError=true for exit 1")
	}
}

func TestBashTool_InvalidJSON(t *testing.T) {
	tool := tools.NewBashTool(10, nil)
	result := tool.Execute(context.Background(), json.RawMessage(`not-json`))
	if !result.IsError {
		t.Error("expected error for invalid JSON input")
	}
}

func TestBashTool_EnvVar(t *testing.T) {
	tool := tools.NewBashTool(10, nil)
	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo $HOME",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// $HOME should expand to something non-empty
	if strings.TrimSpace(result.Output) == "" {
		t.Error("expected non-empty $HOME")
	}
}

// ── FileWriteTool ─────────────────────────────────────────────────────────────

func TestFileWriteTool_CreateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	tool := tools.NewFileWriteTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path":    path,
		"content": "hello from test",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "hello from test" {
		t.Errorf("expected 'hello from test', got: %q", string(data))
	}
}

func TestFileWriteTool_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "deep.txt")
	tool := tools.NewFileWriteTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path":    path,
		"content": "deep content",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "deep content" {
		t.Errorf("expected 'deep content', got: %q", string(data))
	}
}

func TestFileWriteTool_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.txt")
	os.WriteFile(path, []byte("original"), 0644)

	tool := tools.NewFileWriteTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path":    path,
		"content": "replaced",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "replaced" {
		t.Errorf("expected 'replaced', got: %q", string(data))
	}
}

func TestFileWriteTool_ReportsBytes(t *testing.T) {
	dir := t.TempDir()
	tool := tools.NewFileWriteTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path":    filepath.Join(dir, "size.txt"),
		"content": "12345",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "5") {
		t.Errorf("expected byte count in output, got: %q", result.Output)
	}
}

// ── FileReadTool ──────────────────────────────────────────────────────────────

func TestFileReadTool_ReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "read_test.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	tool := tools.NewFileReadTool()
	input, _ := json.Marshal(map[string]interface{}{"path": path})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	for _, want := range []string{"line1", "line2", "line3"} {
		if !strings.Contains(result.Output, want) {
			t.Errorf("expected %q in output, got: %q", want, result.Output)
		}
	}
}

func TestFileReadTool_WithOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offset.txt")
	os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0644)

	tool := tools.NewFileReadTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path":   path,
		"offset": 3,
		"limit":  2,
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "c") {
		t.Errorf("expected 'c' in offset output, got: %q", result.Output)
	}
	if strings.Contains(result.Output, "\ta\n") {
		t.Errorf("should not contain 'a' when offset=3, got: %q", result.Output)
	}
}

func TestFileReadTool_MissingFile(t *testing.T) {
	tool := tools.NewFileReadTool()
	input, _ := json.Marshal(map[string]interface{}{
		"path": "/nonexistent/path/does_not_exist_xyz.txt",
	})
	result := tool.Execute(context.Background(), input)
	if !result.IsError {
		t.Error("expected IsError=true for missing file")
	}
}

func TestFileReadTool_LineNumbers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "numbered.txt")
	os.WriteFile(path, []byte("first\nsecond\n"), 0644)

	tool := tools.NewFileReadTool()
	input, _ := json.Marshal(map[string]interface{}{"path": path})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	// FileReadTool adds line numbers
	if !strings.Contains(result.Output, "1") {
		t.Errorf("expected line number '1' in output, got: %q", result.Output)
	}
}

// ── GlobTool ──────────────────────────────────────────────────────────────────

func TestGlobTool_FindGoFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("text"), 0644)

	tool := tools.NewGlobTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.go",
		"path":    dir,
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "a.go") || !strings.Contains(result.Output, "b.go") {
		t.Errorf("expected .go files in output, got: %q", result.Output)
	}
	if strings.Contains(result.Output, "c.txt") {
		t.Errorf("should not contain c.txt, got: %q", result.Output)
	}
}

func TestGlobTool_NoMatches(t *testing.T) {
	dir := t.TempDir()
	tool := tools.NewGlobTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "*.xyz_nonexistent",
		"path":    dir,
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "no matches") {
		t.Errorf("expected 'no matches' for empty dir, got: %q", result.Output)
	}
}

func TestGlobTool_RecursiveDoublestar(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "root.go"), []byte(""), 0644)
	os.WriteFile(filepath.Join(sub, "nested.go"), []byte(""), 0644)

	tool := tools.NewGlobTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "**/*.go",
		"path":    dir,
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "root.go") || !strings.Contains(result.Output, "nested.go") {
		t.Errorf("expected both root.go and nested.go, got: %q", result.Output)
	}
}

// ── GrepTool ──────────────────────────────────────────────────────────────────

func TestGrepTool_FindPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "fruits.txt"),
		[]byte("apple\nbanana\ncherry\napricot\n"), 0644)

	tool := tools.NewGrepTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "ap",
		"path":    dir,
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "apple") {
		t.Errorf("expected 'apple' in output, got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "apricot") {
		t.Errorf("expected 'apricot' in output, got: %q", result.Output)
	}
	if strings.Contains(result.Output, "banana") {
		t.Errorf("should not match 'banana', got: %q", result.Output)
	}
}

func TestGrepTool_NoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world\n"), 0644)

	tool := tools.NewGrepTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "zzz_nomatch_zzz",
		"path":    dir,
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "no matches") {
		t.Errorf("expected 'no matches', got: %q", result.Output)
	}
}

func TestGrepTool_InvalidRegex(t *testing.T) {
	dir := t.TempDir()
	tool := tools.NewGrepTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "[invalid regex((",
		"path":    dir,
	})
	result := tool.Execute(context.Background(), input)
	if !result.IsError {
		t.Error("expected error for invalid regex")
	}
}

func TestGrepTool_WithIncludeFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("func not here\n"), 0644)

	tool := tools.NewGrepTool()
	input, _ := json.Marshal(map[string]interface{}{
		"pattern": "func",
		"path":    dir,
		"include": "*.go",
	})
	result := tool.Execute(context.Background(), input)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "code.go") {
		t.Errorf("expected code.go in output, got: %q", result.Output)
	}
}

// ── Registry ──────────────────────────────────────────────────────────────────

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.NewBashTool(10, nil))
	reg.Register(tools.NewFileReadTool())
	reg.Register(tools.NewFileWriteTool())

	bash, err := reg.Get("bash")
	if err != nil {
		t.Fatalf("expected to find bash tool: %v", err)
	}
	if bash.Name() != "bash" {
		t.Errorf("expected name 'bash', got %q", bash.Name())
	}

	_, err = reg.Get("nonexistent_tool_xyz")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

func TestRegistry_APIDefs(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.NewBashTool(10, nil))
	reg.Register(tools.NewFileReadTool())
	reg.Register(tools.NewGlobTool())

	defs := reg.APIDefs()
	if len(defs) != 3 {
		t.Errorf("expected 3 tool defs, got %d", len(defs))
	}
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, want := range []string{"bash", "file_read", "glob"} {
		if !names[want] {
			t.Errorf("expected tool %q in APIDefs", want)
		}
	}
}

func TestRegistry_OrderPreserved(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.NewGlobTool())
	reg.Register(tools.NewGrepTool())
	reg.Register(tools.NewBashTool(10, nil))

	defs := reg.APIDefs()
	if len(defs) != 3 {
		t.Fatalf("expected 3 defs, got %d", len(defs))
	}
	if defs[0].Name != "glob" || defs[1].Name != "grep" || defs[2].Name != "bash" {
		t.Errorf("registration order not preserved: %v", []string{defs[0].Name, defs[1].Name, defs[2].Name})
	}
}
