<#
.SYNOPSIS
  Runs the Steamscope scraper once (games, bundles, price-history backfill, cleanup).

.DESCRIPTION
  Called by the Windows scheduled task created with register-scrape-task.ps1.
  Writes a dated log under <RepoRoot>\logs, keeps the last 14, prevents two runs
  from overlapping, and exits non-zero on failure so Task Scheduler shows it.

.PARAMETER RepoRoot
  The repository root (it contains .env and backend\).
#>
param(
  [string]$RepoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path,
  [int]$KeepLogs = 14
)

$ErrorActionPreference = 'Stop'
$logDir = Join-Path $RepoRoot 'logs'
New-Item -ItemType Directory -Force -Path $logDir | Out-Null
$logFile = Join-Path $logDir ("scrape-{0}.log" -f (Get-Date -Format 'yyyy-MM-dd'))

function Write-Log([string]$message) {
  $line = '{0} {1}' -f (Get-Date -Format 'o'), $message
  Add-Content -Path $logFile -Value $line
  Write-Output $line
}

# A named mutex stops a slow run from overlapping the next one.
$mutex = New-Object System.Threading.Mutex($false, 'Global\SteamscopeScrape')
if (-not $mutex.WaitOne(0)) {
  Write-Log 'another scrape is still running; skipping this run'
  exit 0
}

try {
  $binary = Join-Path $RepoRoot 'scraper.exe'
  if (-not (Test-Path $binary)) {
    Write-Log "scraper.exe not found at $binary; building it"
    Push-Location $RepoRoot
    try { go build -o scraper.exe ./backend/cmd/scraper; if ($LASTEXITCODE -ne 0) { throw 'go build failed' } }
    finally { Pop-Location }
  }

  Write-Log 'scrape starting'
  $stopwatch = [System.Diagnostics.Stopwatch]::StartNew()
  # Run from the repo root so the scraper finds .env and the relative cookie path.
  Push-Location $RepoRoot
  try {
    & $binary *>> $logFile
    $code = $LASTEXITCODE
  } finally { Pop-Location }

  if ($code -ne 0) {
    Write-Log ("scrape FAILED (exit {0}) after {1:n0}s" -f $code, $stopwatch.Elapsed.TotalSeconds)
    exit $code
  }
  Write-Log ("scrape finished OK in {0:n0}s" -f $stopwatch.Elapsed.TotalSeconds)

  Get-ChildItem $logDir -Filter 'scrape-*.log' | Sort-Object LastWriteTime -Descending |
    Select-Object -Skip $KeepLogs | Remove-Item -Force
} finally {
  $mutex.ReleaseMutex()
  $mutex.Dispose()
}
