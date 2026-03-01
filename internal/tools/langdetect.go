package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// DetectLangFromCommand guesses a syntax-highlighting language from a bash command string.
func DetectLangFromCommand(cmd string) string {
	lower := strings.ToLower(strings.TrimSpace(cmd))

	switch {
	case hasAnyPrefix(lower, "python", "python3"):
		return "python"
	case hasAnyPrefix(lower, "node ", "node\t", "nodejs"):
		return "javascript"
	case hasAnyPrefix(lower, "ruby "):
		return "ruby"
	case hasAnyPrefix(lower, "go run", "go build", "go test"):
		return "go"
	case strings.Contains(lower, " | jq") || strings.HasPrefix(lower, "jq "):
		return "json"
	case hasAnyPrefix(lower, "curl ", "wget ") && strings.Contains(lower, "json"):
		return "json"
	case hasAnyPrefix(lower, "cat ", "head ", "tail ", "less ", "more "):
		return DetectLangFromPath(strings.Fields(cmd)[len(strings.Fields(cmd))-1])
	case hasAnyPrefix(lower, "grep ", "rg "):
		return "" // mixed output
	case hasAnyPrefix(lower, "docker ", "kubectl "):
		return "yaml"
	}
	return ""
}

// DetectLangFromPath guesses a language from a file extension.
func DetectLangFromPath(path string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "go":
		return "go"
	case "py":
		return "python"
	case "js", "mjs", "cjs":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "rs":
		return "rust"
	case "java":
		return "java"
	case "c", "h":
		return "c"
	case "cpp", "cc", "cxx", "hpp":
		return "cpp"
	case "sh", "bash", "zsh":
		return "bash"
	case "json":
		return "json"
	case "yaml", "yml":
		return "yaml"
	case "toml":
		return "toml"
	case "xml", "html", "htm":
		return "html"
	case "css":
		return "css"
	case "md", "markdown":
		return "markdown"
	case "sql":
		return "sql"
	case "dockerfile":
		return "dockerfile"
	case "tf":
		return "hcl"
	case "rb":
		return "ruby"
	case "php":
		return "php"
	case "swift":
		return "swift"
	case "kt", "kts":
		return "kotlin"
	case "cs":
		return "csharp"
	}
	return ""
}

// DetectLangFromContent tries to identify the language from the output content.
func DetectLangFromContent(s string) string {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 {
		return ""
	}
	// JSON object or array
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		if json.Valid([]byte(trimmed)) {
			return "json"
		}
	}
	// YAML-like (key: value lines)
	lines := strings.SplitN(trimmed, "\n", 5)
	yamlLines := 0
	for _, l := range lines {
		if strings.Contains(l, ": ") || strings.HasPrefix(l, "- ") {
			yamlLines++
		}
	}
	if yamlLines >= 2 && len(lines) >= 2 {
		return "yaml"
	}
	// XML/HTML
	if strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<html") {
		return "xml"
	}
	return ""
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
