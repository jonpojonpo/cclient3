package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// WebSearchTool searches the web using DuckDuckGo's Instant Answer API.
// No API key required.
type WebSearchTool struct {
	timeout time.Duration
}

func NewWebSearchTool() *WebSearchTool {
	return &WebSearchTool{timeout: 10 * time.Second}
}

func (t *WebSearchTool) Name() string { return "web_search" }

func (t *WebSearchTool) Description() string {
	return "Search the web using DuckDuckGo and return relevant results. Returns an instant answer, abstract summary, and related topics with URLs. Use this to find current information, look up documentation, or research any topic."
}

func (t *WebSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			},
			"max_results": {
				"type": "integer",
				"description": "Maximum number of related topics to return (default: 5)"
			}
		},
		"required": ["query"]
	}`)
}

// ddgResponse is the DuckDuckGo Instant Answer API response shape.
type ddgResponse struct {
	Abstract       string `json:"Abstract"`
	AbstractText   string `json:"AbstractText"`
	AbstractURL    string `json:"AbstractURL"`
	AbstractSource string `json:"AbstractSource"`
	Answer         string `json:"Answer"`
	AnswerType     string `json:"AnswerType"`
	Definition     string `json:"Definition"`
	DefinitionURL  string `json:"DefinitionURL"`
	RelatedTopics  []struct {
		Text     string `json:"Text"`
		FirstURL string `json:"FirstURL"`
		// Some entries are topic groups with nested topics
		Topics []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Topics"`
	} `json:"RelatedTopics"`
}

func (t *WebSearchTool) Execute(ctx context.Context, input json.RawMessage) ToolResult {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return ToolResult{Error: fmt.Sprintf("invalid input: %v", err), IsError: true}
	}
	if strings.TrimSpace(params.Query) == "" {
		return ToolResult{Error: "query is required", IsError: true}
	}
	maxResults := params.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	apiURL := "https://api.duckduckgo.com/?q=" + url.QueryEscape(params.Query) +
		"&format=json&no_html=1&skip_disambig=1"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("request error: %v", err), IsError: true}
	}
	req.Header.Set("User-Agent", "cclient3/1.0 (AI agent; +https://github.com/jonpo/cclient3)")
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("search error: %v", err), IsError: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ToolResult{Error: fmt.Sprintf("HTTP %d from DuckDuckGo", resp.StatusCode), IsError: true}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("read error: %v", err), IsError: true}
	}

	var ddg ddgResponse
	if err := json.Unmarshal(body, &ddg); err != nil {
		return ToolResult{Error: fmt.Sprintf("parse error: %v", err), IsError: true}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search: %q\n\n", params.Query))

	hasContent := false

	// Instant answer (calculations, conversions, etc.)
	if ddg.Answer != "" {
		sb.WriteString(fmt.Sprintf("Answer: %s\n\n", ddg.Answer))
		hasContent = true
	}

	// Wikipedia-style abstract
	if ddg.AbstractText != "" {
		source := ddg.AbstractSource
		if source == "" {
			source = "DuckDuckGo"
		}
		sb.WriteString(fmt.Sprintf("Summary (via %s):\n%s\n", source, ddg.AbstractText))
		if ddg.AbstractURL != "" {
			sb.WriteString(fmt.Sprintf("→ %s\n", ddg.AbstractURL))
		}
		sb.WriteString("\n")
		hasContent = true
	}

	// Dictionary definition
	if ddg.Definition != "" {
		sb.WriteString(fmt.Sprintf("Definition: %s\n", ddg.Definition))
		if ddg.DefinitionURL != "" {
			sb.WriteString(fmt.Sprintf("→ %s\n", ddg.DefinitionURL))
		}
		sb.WriteString("\n")
		hasContent = true
	}

	// Related topics
	count := 0
	for _, topic := range ddg.RelatedTopics {
		if count >= maxResults {
			break
		}
		if topic.Text != "" && topic.FirstURL != "" {
			if count == 0 {
				sb.WriteString("Related:\n")
			}
			sb.WriteString(fmt.Sprintf("  • %s\n    %s\n", topic.Text, topic.FirstURL))
			count++
			hasContent = true
		}
		// Handle nested topic groups (e.g. "Films" → list of films)
		for _, sub := range topic.Topics {
			if count >= maxResults {
				break
			}
			if sub.Text != "" && sub.FirstURL != "" {
				if count == 0 {
					sb.WriteString("Related:\n")
				}
				sb.WriteString(fmt.Sprintf("  • %s\n    %s\n", sub.Text, sub.FirstURL))
				count++
				hasContent = true
			}
		}
	}

	if !hasContent {
		sb.WriteString("No instant results found.\n")
		sb.WriteString("Tip: DuckDuckGo's Instant Answer API works best for well-known topics.\n")
		sb.WriteString("Try web_fetch with a specific URL for more detailed information.\n")
	}

	return ToolResult{Output: sb.String()}
}
