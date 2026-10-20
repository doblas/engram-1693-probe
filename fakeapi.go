package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// runFakeAPI serves just enough of the Anthropic Messages API for a headless
// Claude Code run to call the probe's whoami tool once and finish, so CI can
// exercise real hooks and MCP wiring without credentials or model cost.
func runFakeAPI(addr string) {
	trace, _ := os.OpenFile(logPath()+".api", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if trace != nil {
			fmt.Fprintf(trace, "%s %s %d bytes\n", r.Method, r.URL.Path, len(body))
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/count_tokens"):
			writeJSON(w, map[string]any{"input_tokens": 1})
		case strings.HasSuffix(r.URL.Path, "/v1/messages"):
			fakeMessages(w, body, trace)
		default:
			writeJSON(w, map[string]any{})
		}
	})
	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintln(os.Stderr, "fakeapi:", err)
		os.Exit(1)
	}
}

const whoamiTool = "mcp__probe1693__whoami"

type fakeRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
	Tools  []struct {
		Name string `json:"name"`
	} `json:"tools"`
	Messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
}

// nextAction answers the latest prompt with one whoami call (via ToolSearch
// first when Claude defers MCP tools), then plain text. It looks at the last
// assistant turn rather than at tool results: on --resume Claude Code may keep
// the call as "[Tool use interrupted]" with no result, which must not loop.
func nextAction(req fakeRequest) (tool string, input map[string]any) {
	lastAssistant := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "assistant" {
			lastAssistant = string(req.Messages[i].Content)
			break
		}
	}
	has := func(name string) bool {
		for _, t := range req.Tools {
			if t.Name == name {
				return true
			}
		}
		return false
	}
	searched := strings.Contains(lastAssistant, `"name":"ToolSearch"`)
	called := strings.Contains(lastAssistant, `"tool_use"`) || strings.Contains(lastAssistant, "Tool use interrupted")
	if called && !searched {
		return "", nil
	}
	if has(whoamiTool) {
		return whoamiTool, map[string]any{}
	}
	if has("ToolSearch") && !searched {
		return "ToolSearch", map[string]any{"query": "select:" + whoamiTool, "max_results": 1}
	}
	return "", nil
}

func fakeMessages(w http.ResponseWriter, body []byte, trace io.Writer) {
	var req fakeRequest
	json.Unmarshal(body, &req)
	tool, input := nextAction(req)
	if trace != nil {
		fmt.Fprintf(trace, "  tools=%d stream=%v msgs=%d -> %q\n", len(req.Tools), req.Stream, len(req.Messages), tool)
	}
	block := map[string]any{"type": "text", "text": "done"}
	stop := "end_turn"
	if tool != "" {
		block = map[string]any{"type": "tool_use", "id": "toolu_probe_" + tool[:4], "name": tool, "input": input}
		stop = "tool_use"
	}
	usage := map[string]any{"input_tokens": 1, "output_tokens": 1}
	if !req.Stream {
		writeJSON(w, map[string]any{
			"id": "msg_probe", "type": "message", "role": "assistant", "model": req.Model,
			"content": []any{block}, "stop_reason": stop, "stop_sequence": nil, "usage": usage,
		})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	event := func(name string, data map[string]any) {
		data["type"] = name
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
	}
	event("message_start", map[string]any{"message": map[string]any{
		"id": "msg_probe", "type": "message", "role": "assistant", "model": req.Model,
		"content": []any{}, "stop_reason": nil, "stop_sequence": nil, "usage": usage,
	}})
	if tool != "" {
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{
			"type": "tool_use", "id": block["id"], "name": tool, "input": map[string]any{}}})
		in, _ := json.Marshal(input)
		event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "input_json_delta", "partial_json": string(in)}})
	} else {
		event("content_block_start", map[string]any{"index": 0, "content_block": map[string]any{"type": "text", "text": ""}})
		event("content_block_delta", map[string]any{"index": 0, "delta": map[string]any{"type": "text_delta", "text": "done"}})
	}
	event("content_block_stop", map[string]any{"index": 0})
	event("message_delta", map[string]any{"delta": map[string]any{"stop_reason": stop, "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 1}})
	event("message_stop", map[string]any{})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
