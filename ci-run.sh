#!/usr/bin/env bash
# Headless end-to-end run against the fake API: startup, then --resume.
# Uses a throwaway CLAUDE_CONFIG_DIR, so no real login or credentials are touched.
# Usage: ci-run.sh <project-dir>   (after install.ps1 prepared that project)
# Works on Git Bash (Windows), macOS bash 3.2 and Linux.
set -uo pipefail
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exe="$here/probe"
case "${OSTYPE:-}" in msys* | cygwin*) exe="$here/probe.exe" ;; esac
project="$1"

CLAUDE_CONFIG_DIR="$(mktemp -d)"
# Hand native Windows programs a Windows path.
command -v cygpath >/dev/null && CLAUDE_CONFIG_DIR="$(cygpath -w "$CLAUDE_CONFIG_DIR")"
export CLAUDE_CONFIG_DIR
export ANTHROPIC_API_KEY=probe-fake-key ANTHROPIC_BASE_URL=http://127.0.0.1:18693
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1 DISABLE_AUTOUPDATER=1

"$exe" fakeapi 127.0.0.1:18693 &
api=$!
trap 'kill $api 2>/dev/null' EXIT
for _ in $(seq 50); do curl -fs http://127.0.0.1:18693/ >/dev/null 2>&1 && break; sleep 0.2; done

cd "$project"
limit=()
command -v timeout >/dev/null && limit=(timeout 180)
run() {
  ${limit[@]+"${limit[@]}"} claude -p "Call the whoami tool from probe1693." "$@" \
    --allowedTools mcp__probe1693__whoami --output-format json --max-turns 4
}
first="$(run)"
echo "first run: ${first:0:300}"
sid="$(printf '%s' "$first" | sed -n 's/.*"session_id":"\([^"]*\)".*/\1/p')"
if [ -n "$sid" ]; then
  second="$(run --resume "$sid")"
  echo "resume run: ${second:0:300}"
else
  echo "no session_id in first run; skipping --resume"
fi
