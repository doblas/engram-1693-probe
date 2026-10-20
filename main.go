// probe1693 measures how Claude Code wires hooks and stdio MCP servers to its
// own process, to validate the host-process binding proposed for engram #1693.
//
//	probe hook <label>   run as a Claude Code hook (reads the hook payload on stdin)
//	probe mcp            run as a minimal stdio MCP server exposing one tool: whoami
//	probe report         summarize the log into a table to paste in the issue
//	probe fakeapi [addr] stand-in Messages API so CI runs need no credentials
//
// Every record is appended as one JSON line to probe-1693.jsonl next to the
// executable, so the log location does not depend on how the shell maps TEMP.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type procInfo struct {
	PID   int    `json:"pid"`
	PPID  int    `json:"ppid"`
	Name  string `json:"name"`
	Start string `json:"start,omitempty"`
	Note  string `json:"note,omitempty"`
}

type record struct {
	Time      string            `json:"time"`
	OS        string            `json:"os"`
	Kind      string            `json:"kind"` // hook | mcp-start | mcp-call
	Label     string            `json:"label,omitempty"`
	Event     string            `json:"event,omitempty"` // hook_event_name from the payload
	SessionID string            `json:"session_id,omitempty"`
	Source    string            `json:"source,omitempty"`
	PID       int               `json:"pid"`
	PPID      int               `json:"ppid"`
	Ancestry  []procInfo        `json:"ancestry"`
	Env       map[string]string `json:"env"`
}

var envKeys = []string{
	"CLAUDE_PID", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_PROJECT_DIR",
	"MSYSTEM", "SHELL", "CLAUDE_CODE_GIT_BASH_PATH",
	// Set by probe-hook.sh: what a bash hook script like session-start.sh sees.
	"PROBE_BASH_PID", "PROBE_BASH_PID_WINPID", "PROBE_BASH_PPID", "PROBE_BASH_PPID_WINPID",
}

func logPath() string {
	if p := os.Getenv("PROBE_LOG"); p != "" {
		return p
	}
	exe, err := os.Executable()
	if err != nil {
		return "probe-1693.jsonl"
	}
	return filepath.Join(filepath.Dir(exe), "probe-1693.jsonl")
}

func snapshot(kind, label string) record {
	env := map[string]string{}
	for _, k := range envKeys {
		if v, ok := os.LookupEnv(k); ok {
			env[k] = v
		}
	}
	return record{
		Time: time.Now().Format(time.RFC3339Nano), OS: runtime.GOOS,
		Kind: kind, Label: label, PID: os.Getpid(), PPID: os.Getppid(),
		Ancestry: ancestry(os.Getpid(), 8), Env: env,
	}
}

func appendRecord(r record) {
	f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe:", err)
		return
	}
	defer f.Close()
	b, _ := json.Marshal(r)
	f.Write(append(b, '\n'))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: probe hook <label> | probe mcp | probe report")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "hook":
		label := "hook"
		if len(os.Args) > 2 {
			label = os.Args[2]
		}
		runHook(label)
	case "mcp":
		runMCP(os.Stdin, os.Stdout)
	case "report":
		os.Exit(runReport(logPath(), os.Stdout))
	case "fakeapi":
		addr := "127.0.0.1:18693"
		if len(os.Args) > 2 {
			addr = os.Args[2]
		}
		runFakeAPI(addr)
	default:
		fmt.Fprintln(os.Stderr, "unknown mode:", os.Args[1])
		os.Exit(2)
	}
}

// runHook never blocks or denies: it records and lets Claude continue.
func runHook(label string) {
	r := snapshot("hook", label)
	data, _ := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	var payload struct {
		SessionID string `json:"session_id"`
		Event     string `json:"hook_event_name"`
		Source    string `json:"source"`
	}
	if json.Unmarshal(data, &payload) == nil {
		r.SessionID = payload.SessionID
		r.Source = payload.Source
		r.Event = payload.Event
	}
	appendRecord(r)
}

// runMCP speaks just enough newline-delimited JSON-RPC for Claude Code to
// list and call the whoami tool.
func runMCP(in io.Reader, out io.Writer) {
	appendRecord(snapshot("mcp-start", "mcp"))
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil || len(req.ID) == 0 {
			continue // notifications and garbage need no reply
		}
		reply := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		switch req.Method {
		case "initialize":
			var p struct {
				ProtocolVersion string `json:"protocolVersion"`
			}
			json.Unmarshal(req.Params, &p)
			if p.ProtocolVersion == "" {
				p.ProtocolVersion = "2025-06-18"
			}
			reply["result"] = map[string]any{
				"protocolVersion": p.ProtocolVersion,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "probe1693", "version": "1"},
			}
		case "tools/list":
			reply["result"] = map[string]any{"tools": []any{map[string]any{
				"name":        "whoami",
				"description": "Records which process this MCP server runs under. Call it when asked to.",
				"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
			}}}
		case "tools/call":
			r := snapshot("mcp-call", "whoami")
			appendRecord(r)
			text := fmt.Sprintf("probe recorded: pid=%d ppid=%d CLAUDE_CODE_SESSION_ID=%q", r.PID, r.PPID, r.Env["CLAUDE_CODE_SESSION_ID"])
			reply["result"] = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
		case "ping":
			reply["result"] = map[string]any{}
		default:
			reply["error"] = map[string]any{"code": -32601, "message": "method not found"}
		}
		enc.Encode(reply)
	}
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
