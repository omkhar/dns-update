$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path $env:ProgramData ("dns-update-test-" + [guid]::NewGuid().ToString("N"))
$taskName = "dns-update-ci-$PID"
$binaryPath = Join-Path $tempRoot "dns-update.exe"
$configPath = Join-Path $tempRoot "config.json"
$tokenPath = Join-Path $tempRoot "cloudflare.token"
$logPath = Join-Path $tempRoot "dns-update.log"

New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null

Import-Module ScheduledTasks -ErrorAction Stop | Out-Null

function Cleanup {
    try {
        Unregister-ScheduledTask -TaskName $taskName -Confirm:$false -ErrorAction Stop | Out-Null
    } catch {
    }
    Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue
}

function ShowTaskState {
    try {
        Get-ScheduledTask -TaskName $taskName | Format-List *
        Get-ScheduledTaskInfo -TaskName $taskName | Format-List *
    } catch {
    }
}

trap {
    Cleanup
    throw
}

go build -o $binaryPath (Join-Path $repoRoot "cmd/dns-update")

$configJson = @"
{
  "record": {
    "name": "host.example.com.",
    "zone": "example.com.",
    "ttl_seconds": 300
  },
  "provider": {
    "type": "cloudflare",
    "cloudflare": {
      "zone_id": "CLOUDFLARE_ZONE_ID",
      "api_token_file": "$($tokenPath.Replace('\', '\\'))"
    }
  }
}
"@

Set-Content -LiteralPath $configPath -Value $configJson -Encoding utf8
Set-Content -LiteralPath $tokenPath -Value "dummy-token`n" -Encoding utf8

& (Join-Path $repoRoot "deploy/windows/register-scheduled-task.ps1") `
    -TaskName $taskName `
    -BinaryPath $binaryPath `
    -ConfigPath $configPath `
    -TokenPath $tokenPath `
    -LogPath $logPath `
    -IntervalMinutes 1 `
    -Timeout "30s" `
    -ValidateConfig

$deadline = (Get-Date).AddMinutes(3)
while ((Get-Date) -lt $deadline) {
    if (Test-Path -LiteralPath $logPath) {
        $logContent = Get-Content -LiteralPath $logPath -Raw
        if ($logContent -match "config is valid") {
            break
        }
    }
    Start-Sleep -Seconds 5
}

if (-not (Test-Path -LiteralPath $logPath)) {
    ShowTaskState
    throw "scheduled task did not produce a log file"
}

$logContent = Get-Content -LiteralPath $logPath -Raw
if ($logContent -notmatch "config is valid") {
    ShowTaskState
    Write-Host $logContent
    throw "scheduled task did not validate the config successfully"
}

Cleanup
