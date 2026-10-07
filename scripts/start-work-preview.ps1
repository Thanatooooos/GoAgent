param(
    [string]$DatabaseName = 'codex_work_20261001',
    [int]$Port = 9091,
    [string]$Binary = 'tmp/work-preview.exe'
)

$ErrorActionPreference = 'Stop'
$workspaceDirectory = Split-Path -Parent $PSScriptRoot
if ($DatabaseName -notmatch '^codex_work_[a-zA-Z0-9_]+$') {
    throw 'Work preview must use a dedicated codex_work_ database.'
}
$binaryPath = [IO.Path]::GetFullPath((Join-Path $workspaceDirectory $Binary))
if (!(Test-Path -LiteralPath $binaryPath)) { throw 'Build the preview binary before starting it.' }
$previewDirectory = Join-Path $env:TEMP ('codex-work-preview-' + $DatabaseName)
New-Item -ItemType Directory -Force -Path (Join-Path $previewDirectory 'configs') | Out-Null
Copy-Item -LiteralPath (Join-Path $workspaceDirectory 'configs/application.yaml') -Destination (Join-Path $previewDirectory 'configs/application.yaml')
$environmentFile = Join-Path $workspaceDirectory '.env'
if (Test-Path -LiteralPath $environmentFile) {
    Get-Content -LiteralPath $environmentFile | ForEach-Object {
        if ($_ -match '^([A-Z][A-Z0-9_]*)=(.*)$') {
            $configurationName = $matches[1]
            $configurationValue = $matches[2].Trim().Trim('"').Trim("'")
            if ($configurationName -match '^(AI_|SPRING_|APP_|TAVILY_|RAG_|PARSER_|RUSTFS_|MCP_|SERVER_)') {
                [Environment]::SetEnvironmentVariable($configurationName, $configurationValue, 'Process')
            }
        }
    }
}
$env:SPRING_DATASOURCE_URL = "jdbc:postgresql://127.0.0.1:5432/${DatabaseName}?sslmode=disable"
$env:SPRING_DATASOURCE_USERNAME = 'postgres'
$env:SPRING_DATASOURCE_PASSWORD = 'postgres'
$env:SERVER_PORT = [string]$Port
$env:APP_DEMO_MODE = 'false'
$env:APP_EXPECTED_DATABASE = $DatabaseName
$env:APP_DISABLE_SCHEDULED_JOBS = 'true'
$startedPreview = Start-Process -FilePath $binaryPath -WorkingDirectory $previewDirectory -WindowStyle Hidden -RedirectStandardOutput (Join-Path $workspaceDirectory 'tmp/work-preview.out.log') -RedirectStandardError (Join-Path $workspaceDirectory 'tmp/work-preview.err.log') -PassThru
Write-Output ("Work preview PID: $($startedPreview.Id); database: $DatabaseName; port: $Port")
