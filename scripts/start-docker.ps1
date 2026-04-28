param(
  [switch]$Build,
  [switch]$Detached = $true,
  [switch]$Help
)

$ErrorActionPreference = "Stop"
$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

if ($Help) {
  Write-Host "Usage: .\scripts\start-docker.ps1 [-Build] [-Detached]"
  Write-Host "Starts the full Docker stack from docker-compose.yaml."
  exit 0
}

if (-not (Test-Path ".env") -and (Test-Path ".env.example")) {
  Copy-Item ".env.example" ".env"
  Write-Host "Created .env from .env.example. Fill OPENAI_API_KEY/JWT_SECRET before using AI features." -ForegroundColor Yellow
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
  param([int]$Port)
  $container = Get-DockerContainerUsingPort $Port
  if ($container) {
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

function Release-Port {
  param([int]$Port, [string]$Name)
  Stop-DockerContainerUsingPort $Port
  Stop-LocalPortOwner $Port $Name
}

function Stop-ComposeServiceIfRunning {
  param([string]$Service)
  $id = docker compose ps -q $Service 2>$null
  if ($LASTEXITCODE -eq 0 -and $id) {
    Write-Host "Stopping existing Docker service '$Service'..." -ForegroundColor Yellow
    Invoke-Checked "docker" @("compose", "stop", $Service)
  }
}

Invoke-Checked "docker" @("version")
Stop-ComposeServiceIfRunning "worker"
Release-Port 5432 "PostgreSQL"
Release-Port 6380 "Redis"
Release-Port 8081 "API"
Release-Port 3000 "Frontend"

$args = @("compose", "up")
if ($Build) { $args += "--build" }
if ($Detached) { $args += "-d" }

Invoke-Checked "docker" $args

Write-Host ""
Write-Host "NotebookMind Docker stack:" -ForegroundColor Green
Write-Host "  Frontend: http://localhost:3000"
Write-Host "  API:      http://localhost:8081/api/v1"
Write-Host ""
Write-Host "Logs: docker compose logs -f"
Write-Host "Stop: docker compose down"
