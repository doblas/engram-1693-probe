package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type host struct {
	PID   int
	Start string
	Name  string
}

func (h host) String() string { return fmt.Sprintf("%s pid=%d", h.Name, h.PID) }

func same(p procInfo, h host) bool {
	return p.PID == h.PID && (p.Start == "" || h.Start == "" || p.Start == h.Start)
}

func parentOf(r record) host {
	if len(r.Ancestry) > 1 {
		a := r.Ancestry[1]
		return host{PID: a.PID, Start: a.Start, Name: a.Name}
	}
	return host{PID: r.PPID, Name: "?"}
}

func chain(r record) string {
	names := make([]string, 0, len(r.Ancestry))
	for _, a := range r.Ancestry {
		names = append(names, fmt.Sprintf("%s(%d)", a.Name, a.PID))
	}
	return strings.Join(names, " ← ")
}

func mark(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8] + "…"
	}
	if id == "" {
		return "–"
	}
	return id
}

// tally counts how many applicable records satisfied one check.
type tally struct{ ok, total int }

func (t *tally) add(ok bool) {
	t.total++
	if ok {
		t.ok++
	}
}

func (t tally) verdict() string {
	switch {
	case t.total == 0:
		return "not observed"
	case t.ok == t.total:
		return fmt.Sprintf("✅ holds (%d/%d)", t.ok, t.total)
	default:
		return fmt.Sprintf("❌ fails (%d/%d)", t.ok, t.total)
	}
}

func runReport(path string, out io.Writer) int {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe report:", err)
		return 1
	}
	defer f.Close()
	var recs []record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			recs = append(recs, r)
		}
	}
	var hosts []host
	for _, r := range recs {
		if r.Kind == "mcp-start" {
			hosts = append(hosts, parentOf(r))
		}
	}
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "probe report: no MCP server start recorded; is the probe MCP server connected?")
		return 1
	}
	osName := recs[0].OS

	// hostFor finds the MCP host present in a hook's ancestry, if any.
	hostFor := func(r record) (host, int) {
		for _, h := range hosts {
			for i, a := range r.Ancestry {
				if same(a, h) {
					return h, i
				}
			}
		}
		return hosts[0], -1
	}

	var ppidScript, winpidScript, exeParent, execParent, claudePID, mcpStable tally
	fmt.Fprintf(out, "### Probe results (%s)\n\n", osName)
	fmt.Fprintln(out, "**MCP server**")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| Record | MCP pid | Parent (host) | `CLAUDE_CODE_SESSION_ID` | `CLAUDE_PID` | Chain |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|")
	var current host
	for _, r := range recs {
		if r.Kind != "mcp-start" && r.Kind != "mcp-call" {
			continue
		}
		p := parentOf(r)
		if r.Kind == "mcp-start" {
			current = p
		} else {
			mcpStable.add(p.PID == current.PID && p.Start == current.Start)
		}
		fmt.Fprintf(out, "| %s | %d | %s | %s | %s | %s |\n", r.Kind, r.PID, p, short(r.Env["CLAUDE_CODE_SESSION_ID"]), orDash(r.Env["CLAUDE_PID"]), chain(r))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "**Hooks** (host = the MCP server's parent)")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "| Hook | Event | Source | Session | Host depth | bash `$PPID` = host | winpid(`$PPID`) = host | exe parent = host | `CLAUDE_PID` = host | Chain |")
	fmt.Fprintln(out, "|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range recs {
		if r.Kind != "hook" {
			continue
		}
		h, depth := hostFor(r)
		depthText := "not an ancestor"
		if depth >= 0 {
			depthText = fmt.Sprint(depth)
		}
		script, winpid := "–", "–"
		if v, ok := r.Env["PROBE_BASH_PPID"]; ok {
			ok := atoi(v) == h.PID
			ppidScript.add(ok)
			script = fmt.Sprintf("%s %s", mark(ok), v)
			if w := r.Env["PROBE_BASH_PPID_WINPID"]; w != "" {
				ok := atoi(w) == h.PID
				winpidScript.add(ok)
				winpid = fmt.Sprintf("%s %s", mark(ok), w)
			}
		} else if strings.HasSuffix(r.Label, "-exec") {
			// Exec form (`args` set): Claude Code spawns the executable with
			// no shell, so os.Getppid() should be the host.
			execParent.add(r.PPID == h.PID)
		} else {
			// Shell form running an executable directly, like
			// `engram hook claude-pre-tool-use`, sees this as os.Getppid().
			exeParent.add(r.PPID == h.PID)
		}
		cp := "–"
		if v, ok := r.Env["CLAUDE_PID"]; ok {
			ok := atoi(v) == h.PID
			claudePID.add(ok)
			cp = fmt.Sprintf("%s %s", mark(ok), v)
		}
		fmt.Fprintf(out, "| %s | %s | %s | %s | %s | %s | %s | %s %d | %s | %s |\n",
			r.Label, orDash(r.Event), orDash(r.Source), short(r.SessionID), depthText, script, winpid, mark(r.PPID == h.PID), r.PPID, cp, chain(r))
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "**Verdicts for the #1693 design**")
	fmt.Fprintln(out)
	fmt.Fprintf(out, "- `session-start.sh` publishing `ENGRAM_HOST_PID=$PPID`: %s\n", ppidScript.verdict())
	fmt.Fprintf(out, "- Same, translated with `/proc/$PPID/winpid` (MSYS only): %s\n", winpidScript.verdict())
	fmt.Fprintf(out, "- Shell-form hook executable, `os.Getppid()` (engram's PreToolUse today): %s\n", exeParent.verdict())
	fmt.Fprintf(out, "- Exec-form hook (`args`), `os.Getppid()` — proposed: %s\n", execParent.verdict())
	fmt.Fprintf(out, "- `CLAUDE_PID` in the hook env equals the MCP parent: %s\n", claudePID.verdict())
	fmt.Fprintf(out, "- MCP parent identity (pid + start time) stable across calls: %s\n", mcpStable.verdict())
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}
