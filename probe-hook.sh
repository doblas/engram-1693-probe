#!/usr/bin/env bash
# Mirrors plugin/claude-code/scripts/session-start.sh: records what a bash hook
# script sees as $PPID (the value engram #1693 publishes as ENGRAM_HOST_PID),
# and on Git Bash/MSYS2 also the Windows PID behind it via /proc/<pid>/winpid.
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exe="$here/probe"
case "${OSTYPE:-}" in msys* | cygwin*) exe="$here/probe.exe" ;; esac

winpid() { [ -r "/proc/$1/winpid" ] && cat "/proc/$1/winpid" 2>/dev/null; }

PROBE_BASH_PID="$$" \
PROBE_BASH_PID_WINPID="$(winpid $$)" \
PROBE_BASH_PPID="$PPID" \
PROBE_BASH_PPID_WINPID="$(winpid "$PPID")" \
  "$exe" hook "${1:-hook}"
exit 0
