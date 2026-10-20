# engram #1693 probe: Claude Code on Windows

Measures whether Claude Code's hooks and its stdio MCP server share the same host
process on Windows, and which PID a hook can use to name it. The #1693 design
depends on both.

It touches nothing of engram. It only creates a throwaway project and a log file.

## What it answers

| Question | Why it matters |
|---|---|
| Is the MCP server's parent the `claude` process? | `engram mcp` looks up its binding via `os.Getppid()` |
| Does `$PPID` in a bash hook equal that PID? | `session-start.sh` publishes `ENGRAM_HOST_PID=$PPID`. On Git Bash this may be an MSYS PID, or an intermediate `bash.exe` |
| Does `/proc/$PPID/winpid` fix it? | Possible translation if `$PPID` is an MSYS PID |
| Is a direct hook executable a child of `claude`? | The PreToolUse binding uses `os.Getppid()` |
| Does the hook env carry `CLAUDE_PID`? | A shell-independent alternative to `$PPID` |
| Does the MCP parent stay the same across `/clear`? | The binding must follow `/clear` |

## Steps (Windows, Git Bash installed as Claude Code requires)

1. Copy this folder to Windows. You need `probe.exe`, `probe-hook.sh`, `install.ps1` and `README.md`.
2. In PowerShell:
   ```powershell
   cd <folder>
   powershell -ExecutionPolicy Bypass -File .\install.ps1
   cd $env:TEMP\engram-1693-probe-project
   claude
   ```
   If Claude asks to approve the project MCP server `probe1693`, approve it. Close any other Claude windows first, so there is only one host.
3. Inside Claude, type:
   1. `Call the whoami tool from probe1693.`
   2. `/clear`
   3. `Call the whoami tool from probe1693.`
   4. Exit with `/exit`, then run `claude --resume` in the same project, pick the session, and ask for `whoami` again.
4. Back in PowerShell, from the kit folder:
   ```powershell
   .\probe.exe report > report.md
   ```
   Paste `report.md` in the issue, or bring it back here. The raw log is `probe-1693.jsonl`.

## Reading the verdicts

- **`session-start.sh` publishing `$PPID` ✅**: the current PR works as is on Windows.
- **❌, but `winpid` ✅**: `session-start.sh` must translate the MSYS PID with `/proc/$PPID/winpid`.
- **Both ❌, `CLAUDE_PID` ✅**: publish the binding with `CLAUDE_PID` instead of `$PPID`.
- **MCP parent is not `claude` (or not stable)**: the design does not hold on Windows. Declare it as an explicit limit; writes keep today's behavior.

## CI run (no Windows machine needed)

`.github/workflows/probe.yml` runs real Claude Code on GitHub's `windows-latest`,
`macos-latest` and `ubuntu-latest` runners, installed with npm and with the
native installer. `probe fakeapi` serves a local stand-in for the Messages API
(`ANTHROPIC_BASE_URL`), so a headless run calls `whoami` once with no
credentials and no model cost. `ci-run.sh` runs a normal start and then
`--resume`. Each job writes its report to the run summary and uploads the raw log.

Limit: `/clear` cannot be triggered headless, so CI covers startup and resume
only. `/clear` needs the interactive steps above.

## Self-test (Linux/macOS, not evidence)

`./sim.sh` plays the Claude host with a bash process. It checks that the kit and
the report detect both a direct child and an intermediate shell. It says nothing
about Claude Code itself.

Rebuild: `go build -o probe . && GOOS=windows GOARCH=amd64 go build -o probe.exe .`
