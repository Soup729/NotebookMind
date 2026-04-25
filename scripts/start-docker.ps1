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

$args = @("compose", "up")
if ($Build) { $args += "--build" }
if ($Detached) { $args += "-d" }

docker @args

Write-Host ""
Write-Host "NotebookMind Docker stack:" -ForegroundColor Green
Write-Host "  Frontend: http://localhost:3000"
Write-Host "  API:      http://localhost:8081/api/v1"
Write-Host ""
Write-Host "Logs: docker compose logs -f"
Write-Host "Stop: docker compose down"
