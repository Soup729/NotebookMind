param(
  [switch]$SkipDocker,
  [switch]$SkipFrontend,
  [switch]$Install,
  [switch]$Help
)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

if ($Help) {
  Write-Host "Usage: .\scripts\start-dev.ps1 [-SkipDocker] [-SkipFrontend] [-Install]"
  Write-Host "Starts PostgreSQL, Redis, API, worker, and frontend for local development."
  exit 0
}

function Ensure-FileFromExample {
  param([string]$Path, [string]$ExamplePath)
  if (-not (Test-Path $Path) -and (Test-Path $ExamplePath)) {
    Copy-Item $ExamplePath $Path
    Write-Host "Created $Path from $ExamplePath. Please fill required API keys if needed." -ForegroundColor Yellow
  }
}

function Wait-Port {
  param([string]$HostName, [int]$Port, [int]$TimeoutSeconds = 45)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  while ((Get-Date) -lt $deadline) {
    try {
      $client = New-Object Net.Sockets.TcpClient
      $async = $client.BeginConnect($HostName, $Port, $null, $null)
      if ($async.AsyncWaitHandle.WaitOne(1000)) {
        $client.EndConnect($async)
        $client.Close()
        return $true
      }
      $client.Close()
    } catch {}
    Start-Sleep -Seconds 1
  }
  return $false
}

Ensure-FileFromExample ".env" ".env.example"
Ensure-FileFromExample "web/.env.local" "web/.env.example"

New-Item -ItemType Directory -Force -Path "logs" | Out-Null
New-Item -ItemType Directory -Force -Path "tmp/uploads" | Out-Null
New-Item -ItemType Directory -Force -Path "storage/exports" | Out-Null

if (-not $SkipDocker) {
  Write-Host "Starting PostgreSQL and Redis..." -ForegroundColor Cyan
  docker compose up -d postgres redis
}

if ($Install) {
  Write-Host "Installing Python export dependencies..." -ForegroundColor Cyan
  python -m pip install -r scripts/requirements-export.txt
}

if (-not $SkipFrontend -and -not (Test-Path "web/node_modules")) {
  Write-Host "Installing frontend dependencies..." -ForegroundColor Cyan
  Push-Location "web"
  npm install
  Pop-Location
}

if (-not (Wait-Port "127.0.0.1" 5432 45)) {
  Write-Host "Warning: PostgreSQL port 5432 is not reachable yet." -ForegroundColor Yellow
}
if (-not (Wait-Port "127.0.0.1" 6380 45)) {
  Write-Host "Warning: Redis port 6380 is not reachable yet." -ForegroundColor Yellow
}

Write-Host "Starting API on http://localhost:8081 ..." -ForegroundColor Cyan
Start-Process powershell -WorkingDirectory $Root -ArgumentList @(
  "-NoExit",
  "-Command",
  "go run ./cmd/api"
)

Start-Sleep -Seconds 8

Write-Host "Starting worker..." -ForegroundColor Cyan
Start-Process powershell -WorkingDirectory $Root -ArgumentList @(
  "-NoExit",
  "-Command",
  "go run ./cmd/worker"
)

if (-not $SkipFrontend) {
  Write-Host "Starting frontend on http://localhost:3000 ..." -ForegroundColor Cyan
  Start-Process powershell -WorkingDirectory (Join-Path $Root "web") -ArgumentList @(
    "-NoExit",
    "-Command",
    "`$env:NEXT_PUBLIC_API_URL='http://localhost:8081/api/v1'; npm run dev"
  )
}

Write-Host ""
Write-Host "NotebookMind dev stack is starting:" -ForegroundColor Green
Write-Host "  API:      http://localhost:8081/api/v1"
Write-Host "  Frontend: http://localhost:3000"
Write-Host "  Postgres: localhost:5432"
Write-Host "  Redis:    localhost:6380"
Write-Host ""
Write-Host "Close the opened PowerShell windows to stop API/worker/frontend."
