#!/usr/bin/env bash
# Self-test for the probe kit. This bash process plays the Claude host: it
# starts the probe MCP server and runs hooks as its own children, the way
# Claude Code does. It is NOT evidence about Claude Code on any platform.
set -euo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
export PROBE_LOG="$(mktemp -d)/probe-1693.jsonl"
export CLAUDE_PID=$$ CLAUDE_CODE_SESSION_ID=sess-before-clear

coproc MCP { exec "$here/probe" mcp; }
rpc() { printf '%s\n' "$1" >&"${MCP[1]}"; read -r reply <&"${MCP[0]}"; echo "  mcp <- ${reply:0:110}"; }
rpc '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}'
rpc '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'

payload() { printf '{"session_id":"%s","hook_event_name":"%s","source":"%s"}' "$1" "$2" "$3"; }

# Case 1 (what Linux measured): the hook script is a direct child of the host.
"$here/probe-hook.sh" SessionStart <<<"$(payload sess-before-clear SessionStart startup)"
"$here/probe" hook PreToolUse <<<"$(payload sess-before-clear PreToolUse '')"
rpc '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"whoami","arguments":{}}}'

# Case 2 (suspected on Git Bash): an intermediate shell that does not exec.
# The MCP process keeps its launch-time env, so /clear changes only the hooks.
CLAUDE_CODE_SESSION_ID=sess-after-clear bash -c '"$0" SessionStart <<<"$1"; true' \
  "$here/probe-hook.sh" "$(payload sess-after-clear SessionStart clear)"
CLAUDE_CODE_SESSION_ID=sess-after-clear bash -c '"$0" hook PreToolUse <<<"$1"; true' \
  "$here/probe" "$(payload sess-after-clear PreToolUse '')"
rpc '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"whoami","arguments":{}}}'

exec {MCP[1]}>&-
wait "$MCP_PID" 2>/dev/null || true
echo
"$here/probe" report
