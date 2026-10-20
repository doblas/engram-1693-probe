# Prepares a throwaway Claude Code project that wires the probe exactly like
# the engram plugin: a bash SessionStart script (run by Claude through Git
# Bash on Windows), a direct-executable PreToolUse hook, and a stdio MCP server.
#
#   powershell -ExecutionPolicy Bypass -File .\install.ps1 [-Project C:\path\to\empty\dir]
param([string]$Project = (Join-Path ([IO.Path]::GetTempPath()) "engram-1693-probe-project"))
$ErrorActionPreference = "Stop"

$kit = $PSScriptRoot
# Windows PowerShell 5.1 has no $IsWindows, so only an explicit $false means Unix.
$exeName = if ($IsWindows -eq $false) { "probe" } else { "probe.exe" }
$exe = Join-Path $kit $exeName
if (-not (Test-Path $exe)) { throw "$exeName not found next to install.ps1" }

# Git Bash runs hook commands, so hand it forward-slash paths.
$bashKit = $kit -replace '\\', '/'

New-Item -ItemType Directory -Force -Path (Join-Path $Project ".claude") | Out-Null

$settings = [ordered]@{
  hooks = [ordered]@{
    SessionStart = @(@{
      matcher = "startup|resume|clear|fork"
      hooks   = @(@{ type = "command"; command = "`"$bashKit/probe-hook.sh`" SessionStart"; timeout = 10 })
    })
    PreToolUse = @(@{
      matcher = "^mcp__probe1693__whoami$"
      hooks   = @(@{ type = "command"; command = "`"$bashKit/$exeName`" hook PreToolUse"; timeout = 5 })
    })
  }
  permissions = @{ allow = @("mcp__probe1693__whoami") }
  enabledMcpjsonServers = @("probe1693")
}
$mcp = @{ mcpServers = @{ probe1693 = @{ type = "stdio"; command = $exe; args = @("mcp") } } }

# UTF-8 without BOM: Claude Code reads these as JSON.
$utf8 = New-Object System.Text.UTF8Encoding($false)
[IO.File]::WriteAllText((Join-Path $Project ".claude\settings.json"), ($settings | ConvertTo-Json -Depth 10), $utf8)
[IO.File]::WriteAllText((Join-Path $Project ".mcp.json"), ($mcp | ConvertTo-Json -Depth 10), $utf8)

$log = Join-Path $kit "probe-1693.jsonl"
if (Test-Path $log) { Remove-Item $log }

Write-Host "Project ready: $Project"
Write-Host "Log:           $log"
Write-Host "Next: cd `"$Project`"; claude   (then follow README.md, step 3)"
