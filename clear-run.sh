#!/usr/bin/env bash
# Headless /clear: one Claude process receives prompt, /clear, prompt over
# stream-json input. SessionStart(clear) must fire with a new session_id while
# the MCP server (and its parent) stay the same.
# Usage: clear-run.sh <project-dir>   (after install.ps1 prepared that project)
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exe="$here/probe"
case "${OSTYPE:-}" in msys* | cygwin*) exe="$here/probe.exe" ;; esac
project="$1"

CLAUDE_CONFIG_DIR="$(mktemp -d)"
command -v cygpath >/dev/null && CLAUDE_CONFIG_DIR="$(cygpath -w "$CLAUDE_CONFIG_DIR")"
export CLAUDE_CONFIG_DIR
export ANTHROPIC_API_KEY=probe-fake-key ANTHROPIC_BASE_URL=http://127.0.0.1:18693
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_AUTOUPDATER=1 ENABLE_TOOL_SEARCH=false

"$exe" fakeapi 127.0.0.1:18693 &
api=$!
trap 'kill $api 2>/dev/null' EXIT
for _ in $(seq 50); do curl -fs http://127.0.0.1:18693/ >/dev/null 2>&1 && break; sleep 0.2; done

msg() { printf '{"type":"user","message":{"role":"user","content":"%s"}}\n' "$1"; }
cd "$project"
# Feed messages one at a time, waiting for each turn's result before the next.
{
  msg "Call the whoami tool from probe1693."; sleep 15
  msg "/clear"; sleep 15
  msg "Call the whoami tool from probe1693."; sleep 15
} | claude -p --input-format stream-json --output-format stream-json --verbose \
    --allowedTools mcp__probe1693__whoami --max-turns 4 \
  | grep -o '"type":"result"[^}]*\|"subtype":"init"[^,]*,"[^,]*,"session_id":"[^"]*"\|"session_id":"[^"]*"' | sort | uniq -c
