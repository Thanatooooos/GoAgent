# Load retrieve-eval corpus into kb_eval (requires Docker: postgres + object-storage).
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

Set-Location (Join-Path $PSScriptRoot '..')

$env:SPRING_DATASOURCE_USERNAME = 'postgres'
$env:SPRING_DATASOURCE_PASSWORD = 'postgres'
$env:RUSTFS_ACCESS_KEY_ID = 'rustfsadmin'
$env:RUSTFS_SECRET_ACCESS_KEY = 'rustfsadmin'

Write-Host 'Starting dependencies...'
docker compose up -d postgres object-storage object-storage-init

Write-Host 'Loading corpus...'
$chunkCfg = '{"chunkSize":512,"overlapSize":128}'
go run ./cmd/corpus-loader `
  -dir testdata/corpus `
  -kb kb_eval `
  -clean-kb `
  -chunk-strategy fixed_size `
  -chunk-config $chunkCfg `
  -manifest testdata/corpus/manifest.json

Write-Host 'Done. Manifest: testdata/corpus/manifest.json'
