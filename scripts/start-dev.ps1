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

function Invoke-Checked {
  param([string]$File, [string[]]$Arguments)
  & $File @Arguments
  if ($LASTEXITCODE -ne 0) {
    throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
  }
}

function Get-DockerContainerUsingPort {
  param([int]$Port)
  $lines = docker ps --format "{{.Names}}|{{.Ports}}" 2>$null
  if ($LASTEXITCODE -ne 0) {
    return $null
  }
  foreach ($line in $lines) {
    if ($line -match "[:.]$Port->") {
      return ($line -split "\|", 2)[0]
    }
  }
  return $null
}

function Assert-DockerPortAvailable {
  param([int]$Port, [string[]]$AllowedContainers)
  $container = Get-DockerContainerUsingPort $Port
  if ($container -and ($AllowedContainers -notcontains $container)) {
    throw "Port $Port is already used by Docker container '$container'. Stop it first, for example: docker stop $container"
  }
}

function Stop-DockerContainerUsingPort {
  param([int]$Port, [string[]]$AllowedContainers)
  $container = Get-DockerContainerUsingPort $Port
  if ($container -and ($AllowedContainers -notcontains $container)) {
    Write-Host "Stopping Docker container '$container' using port $Port..." -ForegroundColor Yellow
    Invoke-Checked "docker" @("stop", $container)
  }
}

function Stop-LocalPortOwner {
  param([int]$Port, [string]$Name)
  $listeners = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
  if (-not $listeners) { return }

  $owners = $listeners | Select-Object -ExpandProperty OwningProcess -Unique
  foreach ($owner in $owners) {
    $process = Get-Process -Id $owner -ErrorAction SilentlyContinue
    if (-not $process) { continue }
    Write-Host "Stopping $Name port $Port owner: $($process.ProcessName) (pid $owner)..." -ForegroundColor Yellow
    Stop-Process -Id $owner -Force -ErrorAction SilentlyContinue
  }
  Start-Sleep -Seconds 1
}

function Stop-ComposeServiceIfRunning {
  param([string]$Service)
  $id = docker compose ps -q $Service 2>$null
  if ($LASTEXITCODE -eq 0 -and $id) {
    Write-Host "Stopping existing Docker service '$Service'..." -ForegroundColor Yellow
    Invoke-Checked "docker" @("compose", "stop", $Service)
  }
}

function Assert-LocalPortAvailable {
  param([int]$Port, [string]$Name)
  $listeners = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
  if ($listeners) {
    $owners = $listeners | Select-Object -ExpandProperty OwningProcess -Unique
    $processes = $owners | ForEach-Object {
      Get-Process -Id $_ -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty ProcessName
    }
    throw "$Name port $Port is already in use by process/container: $($processes -join ', '). Stop it first or use the Docker stack only."
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
  Invoke-Checked "docker" @("version")
  Stop-ComposeServiceIfRunning "worker"
  Stop-DockerContainerUsingPort 5432 @("notebookmind_postgres")
  Stop-DockerContainerUsingPort 6380 @("notebookmind_redis")
  Stop-DockerContainerUsingPort 8081 @()
  if (-not $SkipFrontend) {
    Stop-DockerContainerUsingPort 3000 @()
  }
  Write-Host "Starting PostgreSQL and Redis..." -ForegroundColor Cyan
  Invoke-Checked "docker" @("compose", "up", "-d", "postgres", "redis")
}

if ($Install) {
  Write-Host "Installing Python export dependencies..." -ForegroundColor Cyan
  Invoke-Checked "python" @("-m", "pip", "install", "-r", "scripts/requirements-export.txt")
}

if (-not $SkipFrontend -and -not (Test-Path "web/node_modules")) {
  Write-Host "Installing frontend dependencies..." -ForegroundColor Cyan
  Push-Location "web"
  if (Test-Path "package-lock.json") {
    Invoke-Checked "npm" @("ci")
  } else {
    Invoke-Checked "npm" @("install")
  }
  Pop-Location
}

if (-not (Wait-Port "127.0.0.1" 5432 45)) {
  throw "PostgreSQL port 5432 is not reachable. Check Docker Desktop and run: docker compose logs postgres"
}
if (-not (Wait-Port "127.0.0.1" 6380 45)) {
  throw "Redis port 6380 is not reachable. Check Docker Desktop and run: docker compose logs redis"
}

Stop-LocalPortOwner 8081 "API"
if (-not $SkipFrontend) {
  Stop-LocalPortOwner 3000 "Frontend"
}
Assert-LocalPortAvailable 8081 "API"
if (-not $SkipFrontend) {
  Assert-LocalPortAvailable 3000 "Frontend"
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
