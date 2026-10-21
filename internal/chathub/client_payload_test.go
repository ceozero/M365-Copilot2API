package chathub

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatPayloadCodeInterpreterOptions(t *testing.T) {
	tests := []struct {
		name     string
		req      Request
		disabled bool
	}{
		{name: "ordinary chat", req: Request{Text: "hello"}},
		{name: "explicit switch", req: Request{Text: "write a file", DisableCodeInterpreter: true}, disabled: true},
		{name: "caller tools", req: Request{Text: "write a file", Tools: []Tool{{Type: "function", Function: json.RawMessage(`{"name":"write_file","parameters":{"type":"object"}}`)}}}, disabled: true},
		{name: "router prompt", req: Request{Text: "You are a tool selection assistant."}, disabled: true},
		{name: "continuation", req: Request{Text: "summarize tool results", ConversationID: "conversation-1", SessionID: "session-1", DisableCodeInterpreter: true}, disabled: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var payload struct {
				Arguments []struct {
					OptionsSets []string `json:"optionsSets"`
				} `json:"arguments"`
			}
			if err := json.NewDecoder(strings.NewReader(chatPayload(test.req, "request-1", test.req.ConversationID == ""))).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Arguments) != 1 {
				t.Fatalf("unexpected arguments: %+v", payload.Arguments)
			}
			hasInterpreter := false
			hasUpload := false
			for _, option := range payload.Arguments[0].OptionsSets {
				if strings.Contains(option, "code_interpreter") {
					hasInterpreter = true
					if test.disabled {
						t.Errorf("disabled code interpreter option leaked: %s", option)
					}
				}
				if option == "cwc_fileupload_odb" {
					hasUpload = true
				}
			}
			if !test.disabled && !hasInterpreter {
				t.Error("ordinary chat lost code interpreter options")
			}
			if !hasUpload {
				t.Error("file upload option was unexpectedly removed")
			}
		})
	}
}

func TestToolProtocolPromptDescribesCallerExecutionAndSchema(t *testing.T) {
	tools := []Tool{{Type: "function", Function: json.RawMessage(`{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"},"days":{"type":"integer"}},"required":["city","days"]}}`)}}
	prompt := toolProtocolPrompt("Weather in Berlin", tools, "required", false)
	for _, expected := range []string{
		"external caller tools",
		"workspace paths",
		"inspect it with the declared read/search/list tools",
		"Never claim to have read files without successful tool results",
		"not in your cloud runtime",
		"fenced tool_call JSON block",
		`tool_choice is "required"`,
		`"days":{"type":"integer"}`,
		`"required":["city","days"]`,
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("tool protocol prompt is missing %q", expected)
		}
	}
}