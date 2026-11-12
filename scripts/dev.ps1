param(
  [int]$ApiPort = 62943,
  [int]$NatsPort = 9820
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

New-Item -ItemType Directory -Force -Path "$root/data/dev" | Out-Null
$env:FASTEMS_API = "http://127.0.0.1:$ApiPort"

$air = Get-Command air -ErrorAction SilentlyContinue
if ($air) {
  Write-Host "Starting Go backend with air hot reload on port $ApiPort..."
  Start-Process powershell -ArgumentList @(
    "-NoExit", "-Command",
    "Set-Location '$root'; `$env:FASTEMS_API='$env:FASTEMS_API'; air"
  ) | Out-Null
} else {
  if (-not (Test-Path "$root/fastiot.exe")) {
    throw "fastiot.exe not found. Build it first, or install air: go install github.com/air-verse/air@latest"
  }
  Write-Host "air not found; starting the existing fastiot.exe. Install air for Go hot reload."
  Start-Process "$root/fastiot.exe" -ArgumentList @(
    "-port", "$ApiPort", "-nats-port", "$NatsPort", "-data", "$root/data/dev"
  ) | Out-Null
}

Write-Host "Starting Vite dev server on http://127.0.0.1:5173 (API proxy: $env:FASTEMS_API)"
Set-Location "$root/frontend"
npm run dev
