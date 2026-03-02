package agent

import (
	"encoding/json"
	"testing"

	"github.com/jonpo/cclient3/internal/api"
)

// --- safeRawJSON ---

func TestSafeRawJSON_ValidJSON(t *testing.T) {
	cases := []string{
		`{}`,
		`{"key":"value"}`,
		`{"n":42,"arr":[1,2,3]}`,
	}
	for _, tc := range cases {
		got := safeRawJSON(tc)
		if !json.Valid(got) {
			t.Errorf("safeRawJSON(%q) produced invalid JSON: %s", tc, got)
		}
		if string(got) != tc {
			t.Errorf("safeRawJSON(%q) = %q, want same", tc, got)
		}
	}
}

func TestSafeRawJSON_Empty(t *testing.T) {
	got := safeRawJSON("")
	if string(got) != "{}" {
		t.Errorf("safeRawJSON(\"\") = %q, want {}", got)
	}
}

func TestSafeRawJSON_InvalidJSON(t *testing.T) {
	got := safeRawJSON("{broken json")
	if string(got) != "{}" {
		t.Errorf("safeRawJSON(invalid) = %q, want {}", got)
	}
}

// --- markLastBlock ---

func TestMarkLastBlock_StringContent(t *testing.T) {
	msg := api.Message{Role: "user", Content: "hello"}
	markLastBlock(&msg)

	blocks, ok := msg.Content.([]api.ContentBlock)
	if !ok {
		t.Fatalf("expected []ContentBlock after markLastBlock on string content, got %T", msg.Content)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].CacheControl == nil {
		t.Error("expected CacheControl to be set on last block")
	}
	if blocks[0].Text != "hello" {
		t.Errorf("expected text %q, got %q", "hello", blocks[0].Text)
	}
}

func TestMarkLastBlock_BlocksContent(t *testing.T) {
	msg := api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "text", Text: "first"},
			{Type: "text", Text: "last"},
		},
	}
	markLastBlock(&msg)

	blocks := msg.Content.([]api.ContentBlock)
	if blocks[0].CacheControl != nil {
		t.Error("first block should NOT have CacheControl")
	}
	if blocks[1].CacheControl == nil {
		t.Error("last block should have CacheControl")
	}
}

// --- copyMessagesWithCache ---

func TestCopyMessagesWithCache_StripsPreviousMarkers(t *testing.T) {
	ephemeral := &api.CacheControl{Type: "ephemeral"}
	orig := []api.Message{
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "text", Text: "first", CacheControl: ephemeral},
			},
		},
		{
			Role: "assistant",
			Content: []api.ContentBlock{
				{Type: "text", Text: "reply"},
			},
		},
		{
			Role: "user",
			Content: []api.ContentBlock{
				{Type: "text", Text: "second"},
			},
		},
	}

	result := copyMessagesWithCache(orig)

	// Verify original is not mutated
	if orig[0].Content.([]api.ContentBlock)[0].CacheControl == nil {
		t.Error("original first message should retain its CacheControl (not mutated)")
	}

	// The penultimate user message (index 0 = "first") should get a breakpoint
	blocks0 := result[0].Content.([]api.ContentBlock)
	if blocks0[0].CacheControl == nil {
		t.Error("expected breakpoint on penultimate user message")
	}

	// The last user message should NOT have a breakpoint (it's the current turn)
	blocks2 := result[2].Content.([]api.ContentBlock)
	if blocks2[0].CacheControl != nil {
		t.Error("last user message should not have CacheControl set by copyMessagesWithCache")
	}
}

func TestCopyMessagesWithCache_SingleUserMessage(t *testing.T) {
	orig := []api.Message{
		{Role: "user", Content: "only message"},
	}
	// Should not panic; only 1 user message so no penultimate to mark
	result := copyMessagesWithCache(orig)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestCopyMessagesWithCache_DoesNotMutateOriginal(t *testing.T) {
	orig := []api.Message{
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "a"}}},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "b"}}},
		{Role: "user", Content: []api.ContentBlock{{Type: "text", Text: "c"}}},
	}
	copyMessagesWithCache(orig)

	// Original blocks should be unmodified
	if orig[0].Content.([]api.ContentBlock)[0].CacheControl != nil {
		t.Error("original should not be mutated by copyMessagesWithCache")
	}
}
