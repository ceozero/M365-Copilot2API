package web

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// fenceStream decides how much of the assistant text can be released to the
// client right now and what has to be held back for the next fragment.
//
// A closed fence, JSON object or protocol marker may be a tool call rather than
// prose, so it is held while that is still undecided. `hold` reports whether a
// candidate is a tool call: those stay buffered for the tool parser to consume at
// the end of the stream, everything else is released immediately and in order.
//
// Releasing in order is what keeps a reply intact (issue #102). Holding a fence
// after text that followed it would emit that text first and the fence at the
// end, so two fences end up adjacent as a run of six backticks and the client
// renders garbage.
//
// `tail` is the minimum trailing byte count kept without splitting UTF-8.
// Partial protocol markers are retained even when they exceed that count.
func fenceStream(pending, fragment string, tail int, hold func(fence string) bool) (emitted, next string) {
	remaining := pending + fragment
	var output strings.Builder
	markers := []string{"```", "{", "CALL_TOOL:", "<m365-tool-call>"}
	for remaining != "" {
		start := -1
		marker := ""
		for _, candidate := range markers {
			index := strings.Index(remaining, candidate)
			if candidate == "CALL_TOOL:" {
				for offset := range remaining {
					if offset+len(candidate) <= len(remaining) && strings.EqualFold(remaining[offset:offset+len(candidate)], candidate) {
						index = offset
						break
					}
				}
			}
			if index >= 0 && (start < 0 || index < start) {
				start, marker = index, candidate
			}
		}
		if start < 0 {
			keep := tail
			if output.Len() > 0 {
				keep = 0
			}
			for _, candidate := range markers {
				for size := 1; size < len(candidate) && size <= len(remaining); size++ {
					suffix := remaining[len(remaining)-size:]
					matches := suffix == candidate[:size]
					if candidate == "CALL_TOOL:" {
						matches = strings.EqualFold(suffix, candidate[:size])
					}
					if matches && size > keep {
						keep = size
					}
				}
			}
			cut := len(remaining) - keep
			if cut < 0 {
				cut = 0
			}
			for cut > 0 && !utf8.ValidString(remaining[:cut]) {
				cut--
			}
			for cut > 0 && cut < len(remaining) && !utf8.RuneStart(remaining[cut]) {
				cut--
			}
			output.WriteString(remaining[:cut])
			return output.String(), remaining[cut:]
		}
		output.WriteString(remaining[:start])
		remaining = remaining[start:]
		end := 0
		switch marker {
		case "```":
			searchFrom := 3
			if newline := strings.IndexByte(remaining[3:], '\n'); newline >= 0 {
				bodyStart := 3 + newline + 1
				body := remaining[bodyStart:]
				if strings.HasPrefix(strings.TrimSpace(body), "{") {
					decoder := json.NewDecoder(strings.NewReader(body))
					var value json.RawMessage
					if err := decoder.Decode(&value); err == nil {
						searchFrom = bodyStart + int(decoder.InputOffset())
					} else if _, invalid := err.(*json.SyntaxError); !invalid {
						return output.String(), remaining
					}
				}
			}
			if closing := strings.Index(remaining[searchFrom:], "```"); closing >= 0 {
				end = searchFrom + closing + 3
			}
		case "{":
			decoder := json.NewDecoder(strings.NewReader(remaining))
			var value json.RawMessage
			if err := decoder.Decode(&value); err == nil {
				end = int(decoder.InputOffset())
			} else if _, invalid := err.(*json.SyntaxError); invalid {
				end = 1
			}
		case "CALL_TOOL:":
			if newline := strings.IndexByte(remaining, '\n'); newline >= 0 {
				end = newline + 1
			}
		case "<m365-tool-call>":
			if closing := strings.Index(remaining, "</m365-tool-call>"); closing >= 0 {
				end = closing + len("</m365-tool-call>")
			}
		}
		if end == 0 || (hold != nil && hold(remaining[:end])) {
			return output.String(), remaining
		}
		output.WriteString(remaining[:end])
		remaining = remaining[end:]
	}
	return output.String(), ""
}

func isToolProtocolText(text string, tools []map[string]any, choice any) bool {
	calls, ok := parseModelToolDecision(text, tools, choice)
	return ok && len(calls) > 0
}
