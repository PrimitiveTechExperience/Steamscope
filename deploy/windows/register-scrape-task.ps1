<#
.SYNOPSIS
  Creates (or replaces) a daily Windows Task Scheduler job that runs the scraper.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File deploy\windows\register-scrape-task.ps1
  powershell -ExecutionPolicy Bypass -File deploy\windows\register-scrape-task.ps1 -At 03:30

.NOTES
  Runs as the current user, only while logged on (no password stored). Remove with:
    Unregister-ScheduledTask -TaskName SteamscopeDailyScrape -Confirm:$false
  Run it on demand with:  Start-ScheduledTask -TaskName SteamscopeDailyScrape
#>
param(
  [string]$At = '04:15',
  [string]$TaskName = 'SteamscopeDailyScrape'
)

$script = Join-Path $PSScriptRoot 'scrape-daily.ps1'
$action = New-ScheduledTaskAction -Execute 'powershell.exe' `
  -Argument "-NoProfile -ExecutionPolicy Bypass -File `"$script`""
$trigger = New-ScheduledTaskTrigger -Daily -At $At
$settings = New-ScheduledTaskSettingsSet `
  -StartWhenAvailable `
  -ExecutionTimeLimit (New-TimeSpan -Hours 2) `
  -MultipleInstances IgnoreNew `
  -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries

Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Settings $settings `
  -Description 'Steamscope: scrape tracked games and bundles, backfill price history' -Force | Out-Null

Write-Output "Registered '$TaskName' to run daily at $At."
Write-Output "Test it now:  Start-ScheduledTask -TaskName $TaskName"
Write-Output "Logs:         $((Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path)\logs"
