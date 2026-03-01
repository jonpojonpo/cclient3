package display

import (
	"embed"
	"sync"

	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/glamour"
)

//go:embed styles/*.json
var styleFS embed.FS

// styleFiles maps theme names to their embedded JSON bytes.
var styleFiles map[string][]byte

func init() {
	styleFiles = make(map[string][]byte)
	for _, name := range []string{"cyber", "ocean", "ember", "mono"} {
		data, err := styleFS.ReadFile("styles/" + name + ".json")
		if err != nil {
			panic("missing embedded style: " + name + ": " + err.Error())
		}
		styleFiles[name] = data
	}
}

// rendererCacheKey uniquely identifies a cached renderer.
type rendererCacheKey struct {
	theme string
	width int
}

// rendererCache caches glamour TermRenderers keyed by theme+width.
type rendererCache struct {
	mu       sync.Mutex
	cache    map[rendererCacheKey]*glamour.TermRenderer
	curTheme string
}

func newRendererCache() *rendererCache {
	return &rendererCache{
		cache: make(map[rendererCacheKey]*glamour.TermRenderer),
	}
}

// Get returns a cached renderer for the given theme and width, creating one on cache miss.
func (rc *rendererCache) Get(themeName string, width int) *glamour.TermRenderer {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// If the theme changed, clear chroma's global registry entry and our cache.
	if themeName != rc.curTheme {
		delete(styles.Registry, "charm")
		rc.cache = make(map[rendererCacheKey]*glamour.TermRenderer)
		rc.curTheme = themeName
	}

	key := rendererCacheKey{theme: themeName, width: width}
	if r, ok := rc.cache[key]; ok {
		return r
	}

	jsonBytes, ok := styleFiles[themeName]
	if !ok {
		// Fall back to cyber if theme not found.
		jsonBytes = styleFiles["cyber"]
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithStylesFromJSONBytes(jsonBytes),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		// If style loading fails, fall back to auto style.
		r, _ = glamour.NewTermRenderer(
			glamour.WithAutoStyle(),
			glamour.WithWordWrap(width),
		)
	}

	rc.cache[key] = r
	return r
}
