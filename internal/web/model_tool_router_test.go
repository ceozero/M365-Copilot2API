package web

import (
	"strings"
	"testing"
)

func TestParseModelToolDecisionAutoAndParallel(t *testing.T) {
	calls, ok := parseModelToolDecision(`{"calls":[{"name":"get_weather","arguments":{"city":"Beijing"}},{"name":"get_time","arguments":{"city":"Beijing"}}]}`, testTools(), "auto")
	if !ok || len(calls) != 2 {
		t.Fatalf("calls=%v ok=%v", calls, ok)
	}
}
func TestParseModelToolDecisionNoCall(t *testing.T) {
	calls, ok := parseModelToolDecision(`{"calls":[]}`, testTools(), "auto")
	if !ok || len(calls) != 0 {
		t.Fatalf("calls=%v ok=%v", calls, ok)
	}
}
func TestModelToolRouterPromptMarksCompletedResults(t *testing.T) {
	p := modelToolRouterPrompt(`assistant tool_calls: [...]
tool[call_x]: 2026-07-18`, testTools(), "auto")
	if !strings.Contains(p, "Completed evidence must not be repeated") || !strings.Contains(p, "tool[call_x]: 2026-07-18") || !strings.Contains(p, "unfinished work remains") {
		t.Fatalf("missing multi-turn evidence constraint: %s", p)
	}
}

func TestParseModelToolDecisionRejectsBadSchema(t *testing.T) {
	calls, ok := parseModelToolDecision("```json\n{\"calls\":[{\"name\":\"get_weather\",\"arguments\":{\"city\":2}}]}\n```", testTools(), "auto")
	if !ok || len(calls) != 0 {
		t.Fatalf("calls=%v ok=%v", calls, ok)
	}
}

func TestToolChoiceRequiresCall(t *testing.T) {
	for _, test := range []struct {
		name     string
		choice   any
		required bool
	}{
		{name: "unspecified"},
		{name: "auto", choice: "auto"},
		{name: "none", choice: "none"},
		{name: "required", choice: "required", required: true},
		{name: "named function", choice: map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}, required: true},
		{name: "legacy named function", choice: map[string]any{"name": "get_weather"}, required: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := toolChoiceRequiresCall(test.choice); got != test.required {
				t.Fatalf("toolChoiceRequiresCall(%v)=%v, want %v", test.choice, got, test.required)
			}
		})
	}
}

func TestModelToolRouterPromptEnforcesNamedTool(t *testing.T) {
	choice := map[string]any{"type": "function", "function": map[string]any{"name": "get_weather"}}
	prompt := modelToolRouterPrompt("Weather in Berlin", testTools(), choice)
	if !strings.Contains(prompt, "A tool call is mandatory") || !strings.Contains(prompt, `"name":"get_weather"`) || !strings.Contains(prompt, "Do not return NO_TOOL_NEEDED") {
		t.Fatalf("missing mandatory named tool selection: %s", prompt)
	}
}

func TestToolRefusalRecognizesUpstreamAccessDenial(t *testing.T) {
	for _, text := range []string{
		"I don\u2019t have access to a `get_weather` tool in this conversation, so I can\u2019t call it for Berlin or verify the current weather.",
		"I do not have access to the get_weather tool.",
	} {
		if !isToolRefusal(text) {
			t.Fatalf("tool refusal was not recognized: %q", text)
		}
	}
	if isToolRefusal("I don't have access to a private document.") {
		t.Fatal("ordinary access denial must not trigger tool recovery")
	}
}
