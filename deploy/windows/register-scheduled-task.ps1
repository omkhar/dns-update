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

$wrapperPath = Join-Path $PSScriptRoot "invoke-dns-update.ps1"
$wrapperPath = (Resolve-Path $wrapperPath).Path
$binaryPath = (Resolve-Path $BinaryPath).Path
$configPath = (Resolve-Path $ConfigPath).Path
$tokenPath = (Resolve-Path $TokenPath).Path
$logPath = [System.IO.Path]::GetFullPath($LogPath)

$logDir = Split-Path -Parent $logPath
if ($logDir) {
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}

$taskRun = @(
    "PowerShell.exe",
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
    $taskRun += "-ValidateConfig"
}

$taskRun = $taskRun -join " "

try {
    & schtasks.exe /Delete /TN $TaskName /F | Out-Null
} catch {
}

& schtasks.exe /Create /F /SC MINUTE /MO $IntervalMinutes /TN $TaskName /TR $taskRun /RU SYSTEM | Out-Null
