package api

import "testing"

func TestApplyModelDefaults_AdaptiveThinking(t *testing.T) {
	cases := []struct {
		model        string
		wantThinking bool
		wantDisplay  string
	}{
		{"claude-opus-4-8", true, "summarized"},
		{"claude-opus-4-7", true, "summarized"},
		{"claude-sonnet-5", true, "summarized"},
		{"claude-fable-5", true, "summarized"},
		{"claude-opus-4-6", true, ""}, // display param not supported on 4.6
		{"claude-sonnet-4-6", true, ""},
		{"claude-haiku-4-5", false, ""}, // no adaptive thinking
		{"claude-haiku-4-5-20251001", false, ""},
		{"gpt-5.4", false, ""},
	}

	for _, tc := range cases {
		req := &Request{Model: tc.model}
		applyModelDefaults(req)
		if got := req.Thinking != nil; got != tc.wantThinking {
			t.Errorf("%s: thinking set = %v, want %v", tc.model, got, tc.wantThinking)
			continue
		}
		if req.Thinking != nil {
			if req.Thinking.Type != "adaptive" {
				t.Errorf("%s: thinking type = %q, want adaptive", tc.model, req.Thinking.Type)
			}
			if req.Thinking.Display != tc.wantDisplay {
				t.Errorf("%s: display = %q, want %q", tc.model, req.Thinking.Display, tc.wantDisplay)
			}
		}
	}
}

func TestApplyModelDefaults_EffortStripped(t *testing.T) {
	// Effort is not supported on Haiku — it must be stripped, not 400.
	req := &Request{Model: "claude-haiku-4-5", OutputConfig: &OutputConfig{Effort: "high"}}
	applyModelDefaults(req)
	if req.OutputConfig != nil {
		t.Error("effort should be stripped for claude-haiku-4-5")
	}

	req = &Request{Model: "claude-opus-4-8", OutputConfig: &OutputConfig{Effort: "high"}}
	applyModelDefaults(req)
	if req.OutputConfig == nil || req.OutputConfig.Effort != "high" {
		t.Error("effort should be preserved for claude-opus-4-8")
	}
}

func TestApplyModelDefaults_ExplicitThinkingPreserved(t *testing.T) {
	req := &Request{Model: "claude-opus-4-8", Thinking: &ThinkingConfig{Type: "adaptive"}}
	applyModelDefaults(req)
	if req.Thinking.Display != "" {
		t.Error("explicit thinking config must not be modified")
	}
}
