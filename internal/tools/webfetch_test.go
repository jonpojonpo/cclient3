package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHtmlToText_StripsTags(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // substring that must appear
		gone  string // substring that must NOT appear
	}{
		{
			name:  "basic tag stripping",
			input: `<html><body><p>Hello world</p></body></html>`,
			want:  "Hello world",
			gone:  "<p>",
		},
		{
			name:  "script block removed",
			input: `<p>Text</p><script>alert('xss')</script><p>After</p>`,
			want:  "After",
			gone:  "alert",
		},
		{
			name:  "style block removed",
			input: `<style>body{color:red}</style><p>Visible</p>`,
			want:  "Visible",
			gone:  "color:red",
		},
		{
			name:  "entity decoding",
			input: `<p>A &amp; B &lt;3 &gt; C &quot;quoted&quot; &nbsp;space</p>`,
			want:  `A & B <3 > C "quoted"`,
			gone:  "&amp;",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := htmlToText(tc.input)
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("want %q in output, got: %q", tc.want, got)
			}
			if tc.gone != "" && strings.Contains(got, tc.gone) {
				t.Errorf("did not want %q in output, got: %q", tc.gone, got)
			}
		})
	}
}

func TestCollapseWhitespace(t *testing.T) {
	input := "line1\n\n\n\nline2\n  line3  \n\n"
	got := collapseWhitespace(input)
	if strings.Count(got, "\n\n") > 1 {
		t.Errorf("expected at most one blank line, got: %q", got)
	}
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("content lines missing from: %q", got)
	}
	if strings.HasSuffix(got, "\n") {
		t.Errorf("expected trimmed output, got trailing newline: %q", got)
	}
}

func TestLooksLikeHTML(t *testing.T) {
	cases := []struct{ input string; want bool }{
		{"<!DOCTYPE html><html>", true},
		{"<html lang=en>", true},
		{"<head><title>", true},
		{`{"json": true}`, false},
		{"plain text", false},
	}
	for _, tc := range cases {
		got := looksLikeHTML(tc.input)
		if got != tc.want {
			t.Errorf("looksLikeHTML(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestWebFetchTool_FetchesURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<!DOCTYPE html><html><head><style>body{color:red}</style></head><body><p>Hello from test server</p><script>evil()</script></body></html>`))
	}))
	defer srv.Close()

	tool := NewWebFetchTool()
	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	result := tool.Execute(context.Background(), input)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Hello from test server") {
		t.Errorf("expected page content in output, got: %q", result.Output)
	}
	if strings.Contains(result.Output, "evil()") {
		t.Errorf("script content leaked into output: %q", result.Output)
	}
	if strings.Contains(result.Output, "color:red") {
		t.Errorf("style content leaked into output: %q", result.Output)
	}
}

func TestWebFetchTool_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tool := NewWebFetchTool()
	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	result := tool.Execute(context.Background(), input)

	if !result.IsError {
		t.Errorf("expected error for 404 response")
	}
	if !strings.Contains(result.Error, "404") {
		t.Errorf("expected 404 in error, got: %q", result.Error)
	}
}

func TestWebFetchTool_MissingURL(t *testing.T) {
	tool := NewWebFetchTool()
	input, _ := json.Marshal(map[string]string{})
	result := tool.Execute(context.Background(), input)
	if !result.IsError {
		t.Errorf("expected error for missing url")
	}
}

func TestWebFetchTool_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Just plain text content here."))
	}))
	defer srv.Close()

	tool := NewWebFetchTool()
	input, _ := json.Marshal(map[string]string{"url": srv.URL})
	result := tool.Execute(context.Background(), input)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Just plain text content here.") {
		t.Errorf("plain text content missing from output: %q", result.Output)
	}
}
