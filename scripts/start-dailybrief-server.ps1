param(
    [Parameter(Mandatory = $true)][string]$DatabaseName,
    [int]$Port = 9091,
    [ValidateSet('mixed', 'scheduled')][string]$Mode = 'scheduled',
    [string]$Binary = 'tmp/dailybrief-migrated-server.exe',
    [switch]$DisableLegacySchedule
)

$ErrorActionPreference = 'Stop'
$workspaceDirectory = Split-Path -Parent $PSScriptRoot
if ($DatabaseName -ne 'ragent' -and $DatabaseName -notmatch '^codex_dailybrief_[a-zA-Z0-9_]+$') {
    throw 'DailyBrief server requires ragent or an explicit codex_dailybrief_ database.'
}
$binaryPath = [IO.Path]::GetFullPath((Join-Path $workspaceDirectory $Binary))
if (!$binaryPath.StartsWith($workspaceDirectory + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'The binary must be inside this workspace.'
}
if (!(Test-Path -LiteralPath $binaryPath)) { throw 'Build the server binary first.' }
if (Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue) {
    throw "Port $Port is already in use; this script never stops an existing service."
}
$runtimeDirectory = Join-Path $workspaceDirectory ('tmp/dailybrief-runtime-' + $DatabaseName)
New-Item -ItemType Directory -Force -Path (Join-Path $runtimeDirectory 'configs') | Out-Null
Copy-Item -LiteralPath (Join-Path $workspaceDirectory 'configs/application.yaml') -Destination (Join-Path $runtimeDirectory 'configs/application.yaml')
$environmentFile = Join-Path $workspaceDirectory '.env'
if (Test-Path -LiteralPath $environmentFile) {
    Get-Content -LiteralPath $environmentFile | ForEach-Object {
        if ($_ -match '^([A-Z][A-Z0-9_]*)=(.*)$') {
            $configurationName = $matches[1]
            $configurationValue = $matches[2].Trim().Trim('"').Trim("'")
            if ($configurationName -match '^(AI_|SPRING_|APP_|TAVILY_|RAG_|PARSER_|RUSTFS_|MCP_|SERVER_)' -and
                [string]::IsNullOrEmpty([Environment]::GetEnvironmentVariable($configurationName, 'Process'))) {
                [Environment]::SetEnvironmentVariable($configurationName, $configurationValue, 'Process')
            }
        }
    }
}
$env:SPRING_DATASOURCE_URL = "jdbc:postgresql://127.0.0.1:5432/${DatabaseName}?sslmode=disable"
$env:SERVER_PORT = [string]$Port
$env:APP_EXPECTED_DATABASE = $DatabaseName
$env:APP_DEMO_MODE = 'false'
$env:APP_DAILY_BRIEF_SCHEDULER = $Mode
$env:APP_DISABLE_SCHEDULED_JOBS = 'false'
$env:APP_DISABLE_LEGACY_DAILY_BRIEF = if ($DisableLegacySchedule) { 'true' } else { 'false' }
# Keep this migration deployment from changing unrelated derived profiles or
# running memory maintenance. Normal server startup keeps its existing defaults.
$env:APP_DISABLE_PROFILE_OBSERVATION = 'true'
$env:RAG_MEMORY_MAINTENANCE_ENABLED = 'false'
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$stdout = Join-Path $runtimeDirectory ("server-$Mode-$stamp.out.log")
$stderr = Join-Path $runtimeDirectory ("server-$Mode-$stamp.err.log")
$startedServer = Start-Process -FilePath $binaryPath -WorkingDirectory $runtimeDirectory -WindowStyle Hidden -RedirectStandardOutput $stdout -RedirectStandardError $stderr -PassThru
[pscustomobject]@{PID=$startedServer.Id;Database=$DatabaseName;Port=$Port;Mode=$Mode;LegacyScheduleDisabled=([bool]$DisableLegacySchedule -or $Mode -eq 'scheduled');Stdout=$stdout;Stderr=$stderr} | ConvertTo-Json
