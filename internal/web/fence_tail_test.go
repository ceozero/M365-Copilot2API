package web

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func collect(t *testing.T, frags []string, hold func(string) bool) string {
	t.Helper()
	pending, out := "", ""
	for _, f := range frags {
		emitted, held := fenceStream(pending, f, 3, hold)
		out += emitted
		pending = held
	}
	return out + pending
}

func neverHold(string) bool { return false }

// A reply containing a code fence used to lose everything after the fence: the
// streaming path emitted the text before the fence, buffered the fence, and
// dropped the tail. That is issue #102 ("truncated at the first fence").
func TestFenceStreamDoesNotDropTheTail(t *testing.T) {
	got := collect(t, []string{
		"I will create the file.\n```bash\n",
		"echo hello > note.txt\n```",
		"Then I will verify it with dir.",
	}, neverHold)

	for _, want := range []string{
		"I will create the file.",
		"Then I will verify it with dir.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output lost %q; got %q", want, got)
		}
	}
}

// A fence split across fragments must be recognised once it closes.
func TestFenceStreamHoldsSplitFence(t *testing.T) {
	got := collect(t, []string{"before ```ba", "sh\necho hi\n``` after"}, neverHold)
	for _, want := range []string{"before ", " after", "echo hi"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost %q; got %q", want, got)
		}
	}
}

// Two consecutive code blocks used to arrive out of order when the model
// streamed them as separate fragments: the first fence was held until the end
// of the stream while the text after it was sent, so the second fence reached
// the client first and the reply rendered as a mangled code section.
func TestFenceStreamKeepsConsecutiveFencesInOrder(t *testing.T) {
	got := collect(t, []string{
		"```text\nHello, World!\n```",
		"\n```python\nprint(1)\n```\nDone.",
	}, neverHold)

	iText := strings.Index(got, "```text")
	iPy := strings.Index(got, "```python")
	if iText < 0 || iPy < 0 {
		t.Fatalf("expected both fences, got %q", got)
	}
	if iText > iPy {
		t.Fatalf("fences out of order:\n%q", got)
	}
	if !strings.Contains(got, "Done.") {
		t.Fatalf("text after the last fence was lost: %q", got)
	}
	// The fences stay separate, each closed, so a client renders two code blocks.
	if !strings.Contains(got, "```text\nHello, World!\n```\n```python") {
		t.Fatalf("fences are not cleanly separated: %q", got)
	}
}

// A fence that is a real tool call must stay buffered so fencedToolCalls can
// consume it, instead of being streamed to the client as prose.
func TestFenceStreamHoldsToolCallFence(t *testing.T) {
	hold := func(fence string) bool {
		return len(fencedToolCalls(fence, []map[string]any{
			{"type": "function", "function": map[string]any{"name": "bash"}},
		}, "auto")) > 0
	}

	pending, out := "", ""
	for _, f := range []string{
		"Running it now.\n```bash\n",
		"{\"command\":\"echo hi\"}\n```",
	} {
		emitted, held := fenceStream(pending, f, 3, hold)
		out += emitted
		pending = held
	}

	if !strings.Contains(out, "Running it now.") {
		t.Fatalf("prose before the tool call was not released: %q", out)
	}
	if strings.Contains(out, "echo hi") {
		t.Fatalf("tool call leaked into the prose stream: %q", out)
	}
	// The fence is still available in pending for fencedToolCalls to consume.
	if !strings.Contains(pending, "echo hi") {
		t.Fatalf("tool call was not held for fencedToolCalls: %q", pending)
	}
	if !strings.Contains(pending, "```bash") {
		t.Fatalf("held text is not a complete fence: %q", pending)
	}
}

// Without the fix, everything after a closed fence was discarded.
func TestFenceStreamRegressionGuard(t *testing.T) {
	out, held := fenceStream("", "a```x\ny\n```TAIL", 3, neverHold)
	if !strings.Contains(out, "TAIL") {
		t.Fatalf("tail dropped: emitted=%q held=%q", out, held)
	}
}

func TestFenceStreamToolProtocolsNeverLeakAcrossFragments(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}}
	hold := func(text string) bool { return isToolProtocolText(text, tools, "auto") }
	for _, protocol := range []string{
		"```tool_call\n{\"name\":\"read_file\",\"arguments\":{\"filePath\":\"/tmp/private.txt\"}}\n```",
		"```json\n{\"calls\":[{\"name\":\"read_file\",\"arguments\":{\"filePath\":\"/tmp/private.txt\"}}]}\n```",
		`{"name":"read_file","arguments":{"filePath":"/tmp/private.txt"}}`,
		`CALL_TOOL: read_file({"filePath":"/tmp/private.txt"})`,
		`call_tool: read_file({"filePath":"/tmp/private.txt"})`,
		`<m365-tool-call>{"name":"read_file","arguments":{"filePath":"/tmp/private.txt"}}</m365-tool-call>`,
	} {
		text := "我先读取文件。ı\n" + protocol + "\nThen continue."
		for split := 0; split <= len(text); split++ {
			pending, output := "", ""
			for _, fragment := range []string{text[:split], text[split:]} {
				emitted, held := fenceStream(pending, fragment, 3, hold)
				output += emitted
				pending = held
				if !utf8.ValidString(emitted) {
					t.Fatalf("invalid UTF-8 at split %d: %q", split, emitted)
				}
			}
			if strings.Contains(output, "private.txt") || strings.Contains(output, "read_file") {
				t.Fatalf("tool protocol leaked at split %d: %q", split, output)
			}
			if !strings.Contains(output, "我先读取文件。") || !strings.Contains(pending, protocol) {
				t.Fatalf("lost prose or tool protocol at split %d: output=%q pending=%q", split, output, pending)
			}
			calls, ok := parseModelToolDecision(pending, tools, "auto")
			if !ok || len(calls) != 1 || calls[0].Name != "read_file" {
				t.Fatalf("held protocol cannot become a tool call: %q", pending)
			}
		}
	}
}

func TestFenceStreamToolProtocolArrivesByteByByte(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}}
	text := "Reading.\ncall_tool: read_file({\"filePath\":\"/tmp/private.txt\"})\n"
	pending, output := "", ""
	for offset := range text {
		emitted, held := fenceStream(pending, text[offset:offset+1], 3, func(candidate string) bool {
			return isToolProtocolText(candidate, tools, "auto")
		})
		output += emitted
		pending = held
	}
	if strings.Contains(output, "read_file") || strings.Contains(output, "private.txt") {
		t.Fatalf("tool protocol leaked: %q", output)
	}
	if calls, ok := parseModelToolDecision(pending, tools, "auto"); !ok || len(calls) != 1 {
		t.Fatalf("tool protocol was lost: %q", pending)
	}
}

func TestFenceStreamToolArgumentsContainBackticks(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "write_file"}}}
	protocol := "```tool_call\n{\"name\":\"write_file\",\"arguments\":{\"content\":\"```json\\nsecret\\n```\"}}\n```"
	for split := 0; split <= len(protocol); split++ {
		pending, output := "", ""
		for _, fragment := range []string{protocol[:split], protocol[split:]} {
			emitted, held := fenceStream(pending, fragment, 3, func(candidate string) bool {
				return isToolProtocolText(candidate, tools, "auto")
			})
			output += emitted
			pending = held
		}
		if output != "" || pending != protocol {
			t.Fatalf("tool arguments leaked at split %d: output=%q pending=%q", split, output, pending)
		}
	}
}

func TestFenceStreamScansToolsAfterOrdinaryCodeBlock(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}}
	text := "```text\nordinary example\n```\n```read_file\n{\"filePath\":\"/tmp/private.txt\"}\n```\n```read_file\n{\"filePath\":\"/tmp/second.txt\"}\n```"
	emitted, held := fenceStream("", text, 3, func(text string) bool { return isToolProtocolText(text, tools, "auto") })
	if !strings.Contains(emitted, "ordinary example") || strings.Contains(emitted, "private.txt") || strings.Contains(emitted, "second.txt") {
		t.Fatalf("unexpected prose: %q", emitted)
	}
	if calls := fencedToolCalls(held, tools, "auto"); len(calls) != 2 {
		t.Fatalf("expected two retained calls, got %d", len(calls))
	}
}

func TestFenceStreamPreservesOrdinaryJSONAndCode(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}}
	hold := func(text string) bool { return isToolProtocolText(text, tools, "auto") }
	for _, text := range []string{
		"普通回答。\n```json\n{\"answer\":42}\n```\nDone.",
		"Result: {\"answer\":42}.\n```text\nhello\n```\nDone.",
		"An undeclared tool example: {\"name\":\"unknown\",\"arguments\":{}}.",
	} {
		for split := 0; split <= len(text); split++ {
			if got := collect(t, []string{text[:split], text[split:]}, hold); got != text {
				t.Fatalf("ordinary content changed at split %d: got=%q want=%q", split, got, text)
			}
		}
	}
}

func TestFenceStreamPreservesProtocolWhenToolsAreDisabled(t *testing.T) {
	tools := []map[string]any{{"type": "function", "function": map[string]any{"name": "read_file"}}}
	text := "Example: {\"name\":\"read_file\",\"arguments\":{\"filePath\":\"example.txt\"}}"
	for split := 0; split <= len(text); split++ {
		got := collect(t, []string{text[:split], text[split:]}, func(candidate string) bool {
			return isToolProtocolText(candidate, tools, "none")
		})
		if got != text {
			t.Fatalf("content changed with tools disabled at split %d: %q", split, got)
		}
	}
}
