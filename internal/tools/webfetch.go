package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

// WebFetchTool fetches a URL and returns its text content.
type WebFetchTool struct {
	timeout time.Duration
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{timeout: 30 * time.Second}
}

func (t *WebFetchTool) Name() string { return "web_fetch" }

func (t *WebFetchTool) Description() string {
	return "Fetch a URL and return its readable text content. HTML pages are automatically converted to plain text. Use this to read documentation, articles, or any web resource."
}

func (t *WebFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch (http or https)"
			},
			"max_bytes": {
				"type": "integer",
				"description": "Maximum bytes to read (default: 102400 = 100KB)"
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		URL      string `json:"url"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}
	if params.URL == "" {
		return ToolResult{Error: "url is required", IsError: true}
	}

	maxBytes := params.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 100 * 1024 // 100KB default
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.URL, nil)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid URL: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", "cclient3/1.0 (AI agent; +https://github.com/jonpo/cclient3)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain,*/*")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("fetch error: %v", err), IsError: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ToolResult{Error: fmt.Sprintf("HTTP %d %s", resp.StatusCode, resp.Status), IsError: true}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("read error: %v", err), IsError: true}
	}

	contentType := resp.Header.Get("Content-Type")
	text := string(body)

	// Convert HTML to readable text
	if strings.Contains(contentType, "html") || looksLikeHTML(text) {
		text = htmlToText(text)
	}

	text = collapseWhitespace(text)
	if text == "" {
		text = "(empty response)"
	}

	return ToolResult{Output: fmt.Sprintf("URL: %s\nStatus: %s\nContent-Type: %s\n\n%s",
		params.URL, resp.Status, contentType, text)}
}

// looksLikeHTML returns true if the content appears to be HTML.
func looksLikeHTML(s string) bool {
	trimmed := strings.TrimSpace(s)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype html") ||
		strings.HasPrefix(lower, "<html") ||
		strings.HasPrefix(lower, "<head")
}

// htmlToText strips HTML markup and returns readable plain text.
// It handles <script>/<style> blocks, inserts newlines at block elements,
// and decodes common HTML entities.
func htmlToText(s string) string {
	var b strings.Builder
	b.Grow(len(s) / 2)

	inTag := false
	inScript := false
	inStyle := false

	i := 0
	for i < len(s) {
		ch := s[i]

		if inTag {
			if ch == '>' {
				inTag = false
			}
			i++
			continue
		}

		if ch == '<' {
			// Peek at tag to decide what to do
			rest := strings.ToLower(s[i:])

			switch {
			case strings.HasPrefix(rest, "<script"):
				inScript = true
			case strings.HasPrefix(rest, "</script"):
				inScript = false
			case strings.HasPrefix(rest, "<style"):
				inStyle = true
			case strings.HasPrefix(rest, "</style"):
				inStyle = false
			default:
				if !inScript && !inStyle {
					// Insert newline before block-level elements
					for _, tag := range []string{"<p", "<br", "<div", "<li", "<tr",
						"<h1", "<h2", "<h3", "<h4", "<h5", "<h6",
						"<article", "<section", "<header", "<footer",
						"</p>", "</div>", "</article>", "</section>"} {
						if strings.HasPrefix(rest, tag) {
							b.WriteByte('\n')
							break
						}
					}
				}
			}
			inTag = true
			i++
			continue
		}

		if !inScript && !inStyle {
			b.WriteByte(ch)
		}
		i++
	}

	result := b.String()

	// Decode common HTML entities
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", `"`)
	result = strings.ReplaceAll(result, "&#39;", "'")
	result = strings.ReplaceAll(result, "&apos;", "'")
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	result = strings.ReplaceAll(result, "&#160;", " ")
	result = strings.ReplaceAll(result, "&mdash;", "—")
	result = strings.ReplaceAll(result, "&ndash;", "–")
	result = strings.ReplaceAll(result, "&hellip;", "…")

	return result
}

// collapseWhitespace trims lines and collapses runs of blank lines to one.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		trimmed := strings.TrimRightFunc(line, unicode.IsSpace)
		if trimmed == "" {
			blank++
			if blank <= 1 {
				out = append(out, "")
			}
		} else {
			blank = 0
			out = append(out, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
