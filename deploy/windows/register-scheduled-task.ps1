param(
    [string]$TaskName = "dns-update",
    [string]$BinaryPath = "C:\Program Files\dns-update\dns-update.exe",
    [string]$ConfigPath = "C:\ProgramData\dns-update\config.json",
    [string]$TokenPath = "C:\ProgramData\dns-update\cloudflare.token",
    [string]$LogPath = "C:\ProgramData\dns-update\dns-update.log",
    [int]$IntervalMinutes = 5,
    [string]$Timeout = "2m",
    [switch]$ValidateConfig
)

$ErrorActionPreference = "Stop"

if ($IntervalMinutes -lt 1 -or $IntervalMinutes -gt 1439) {
    throw "IntervalMinutes must be between 1 and 1439."
}

Import-Module ScheduledTasks -ErrorAction Stop | Out-Null

$wrapperPath = Join-Path $PSScriptRoot "invoke-dns-update.ps1"
$wrapperPath = (Resolve-Path $wrapperPath).Path
$binaryPath = (Resolve-Path $BinaryPath).Path
$configPath = (Resolve-Path $ConfigPath).Path
$tokenPath = (Resolve-Path $TokenPath).Path
$logPath = [System.IO.Path]::GetFullPath($LogPath)
$powerShellPath = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"
$workingDirectory = Split-Path -Parent $binaryPath

$logDir = Split-Path -Parent $logPath
if ($logDir) {
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}

$taskArguments = @(
    "-NoProfile",
    "-NonInteractive",
    "-ExecutionPolicy", "Bypass",
    "-File", ('"{0}"' -f $wrapperPath),
    "-BinaryPath", ('"{0}"' -f $binaryPath),
    "-ConfigPath", ('"{0}"' -f $configPath),
    "-TokenPath", ('"{0}"' -f $tokenPath),
    "-LogPath", ('"{0}"' -f $logPath),
    "-Timeout", ('"{0}"' -f $Timeout)
)

if ($ValidateConfig) {
    $taskArguments += "-ValidateConfig"
}

$action = New-ScheduledTaskAction `
    -Execute $powerShellPath `
    -Argument ($taskArguments -join " ") `
    -WorkingDirectory $workingDirectory
$trigger = New-ScheduledTaskTrigger `
    -Once `
    -At (Get-Date).AddMinutes(1) `
    -RepetitionInterval (New-TimeSpan -Minutes $IntervalMinutes) `
    -RepetitionDuration (New-TimeSpan -Days 3650)
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 10) `
    -MultipleInstances IgnoreNew `
    -StartWhenAvailable

try {
    Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction Stop | Out-Null
} catch {
}

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $action `
    -Trigger $trigger `
    -Principal $principal `
    -Settings $settings `
    -Force | Out-Null
