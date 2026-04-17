$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$tempRoot = Join-Path $env:ProgramData ("dns-update-test-" + [guid]::NewGuid().ToString("N"))
$taskName = "dns-update-ci-$PID"
$binaryPath = Join-Path $tempRoot "dns-update.exe"
$configPath = Join-Path $tempRoot "config.json"
$tokenDir = Join-Path $tempRoot "credentials"
$tokenPath = Join-Path $tokenDir "cloudflare.token"
$logPath = Join-Path $tempRoot "dns-update.log"
$deployRoot = Join-Path $tempRoot "deploy\windows"
$registerScriptPath = Join-Path $deployRoot "register-scheduled-task.ps1"
$invokeScriptPath = Join-Path $deployRoot "invoke-dns-update.ps1"

New-Item -ItemType Directory -Path $tempRoot -Force | Out-Null
New-Item -ItemType Directory -Path $tokenDir -Force | Out-Null
New-Item -ItemType Directory -Path $deployRoot -Force | Out-Null

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
        Get-ScheduledTask -TaskName $taskName | Select-Object -ExpandProperty Actions | Format-List *
        Get-ScheduledTaskInfo -TaskName $taskName | Format-List *
    } catch {
    }
}

function Assert-TokenAcl {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $acl = Get-Acl -LiteralPath $Path
    if (-not $acl.AreAccessRulesProtected) {
        throw "token ACL should disable inheritance"
    }

    $expectedSids = @(
        [System.Security.Principal.SecurityIdentifier]::new([System.Security.Principal.WellKnownSidType]::LocalSystemSid, $null).Value,
        [System.Security.Principal.SecurityIdentifier]::new([System.Security.Principal.WellKnownSidType]::BuiltinAdministratorsSid, $null).Value
    ) | Sort-Object -Unique

    $actualSids = @(
        $acl.Access |
            Where-Object { -not $_.IsInherited -and $_.AccessControlType -eq [System.Security.AccessControl.AccessControlType]::Allow } |
            ForEach-Object { $_.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value }
    ) | Sort-Object -Unique

    if (@(Compare-Object -ReferenceObject $expectedSids -DifferenceObject $actualSids).Count -ne 0) {
        throw ("unexpected token ACL identities: expected [{0}] got [{1}]" -f ($expectedSids -join ", "), ($actualSids -join ", "))
    }
}

function Assert-TokenDirectoryAcl {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $acl = Get-Acl -LiteralPath $Path
    if (-not $acl.AreAccessRulesProtected) {
        throw "token parent directory ACL should disable inheritance"
    }

    $expectedSids = @(
        [System.Security.Principal.SecurityIdentifier]::new([System.Security.Principal.WellKnownSidType]::LocalSystemSid, $null).Value,
        [System.Security.Principal.SecurityIdentifier]::new([System.Security.Principal.WellKnownSidType]::BuiltinAdministratorsSid, $null).Value
    ) | Sort-Object -Unique

    $actualSids = @(
        $acl.Access |
            Where-Object { -not $_.IsInherited -and $_.AccessControlType -eq [System.Security.AccessControl.AccessControlType]::Allow } |
            ForEach-Object { $_.IdentityReference.Translate([System.Security.Principal.SecurityIdentifier]).Value }
    ) | Sort-Object -Unique

    if (@(Compare-Object -ReferenceObject $expectedSids -DifferenceObject $actualSids).Count -ne 0) {
        throw ("unexpected token parent directory ACL identities: expected [{0}] got [{1}]" -f ($expectedSids -join ", "), ($actualSids -join ", "))
    }
}

trap {
    Write-Host ($_ | Out-String)
    Cleanup
    throw
}

go build -o $binaryPath (Join-Path $repoRoot "cmd/dns-update")
Copy-Item (Join-Path $repoRoot "deploy/windows/register-scheduled-task.ps1") $registerScriptPath -Force
Copy-Item (Join-Path $repoRoot "deploy/windows/invoke-dns-update.ps1") $invokeScriptPath -Force

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

& $registerScriptPath `
    -TaskName $taskName `
    -BinaryPath $binaryPath `
    -ConfigPath $configPath `
    -TokenPath $tokenPath `
    -LogPath $logPath `
    -IntervalMinutes 1 `
    -Timeout "30s" `
    -ValidateConfig

Assert-TokenAcl -Path $tokenPath
Assert-TokenDirectoryAcl -Path $tokenDir

$taskArguments = (
    Get-ScheduledTask -TaskName $taskName |
        Select-Object -ExpandProperty Actions |
        Select-Object -ExpandProperty Arguments
)
if ($taskArguments -match "ValidateConfig") {
    ShowTaskState
    throw "scheduled task should not keep ValidateConfig in the installed action"
}

$initialLastRunTime = (Get-ScheduledTaskInfo -TaskName $taskName).LastRunTime

$invalidConfigJson = @"
{
  "record": {
    "name": "host.example.com.",
"@

Set-Content -LiteralPath $configPath -Value $invalidConfigJson -Encoding utf8

$ranAfterRegistration = $false
$deadline = (Get-Date).AddMinutes(3)
while ((Get-Date) -lt $deadline) {
    $taskInfo = Get-ScheduledTaskInfo -TaskName $taskName
    if ($taskInfo.LastRunTime -ne $initialLastRunTime) {
        $ranAfterRegistration = $true
    }

    if (Test-Path -LiteralPath $logPath) {
        $logContent = Get-Content -LiteralPath $logPath -Raw
        if (
            $ranAfterRegistration -and
            $logContent -match "failed to load config: decode config: unexpected EOF" -and
            $logContent -match "exit code: 1"
        ) {
            break
        }
    }
    Start-Sleep -Seconds 5
}

if (-not $ranAfterRegistration) {
    ShowTaskState
    throw "scheduled task did not run after registration"
}

if (-not (Test-Path -LiteralPath $logPath)) {
    ShowTaskState
    throw "scheduled task did not produce a log file"
}

$logContent = Get-Content -LiteralPath $logPath -Raw
if (
    $logContent -notmatch "failed to load config: decode config: unexpected EOF" -or
    $logContent -notmatch "exit code: 1"
) {
    ShowTaskState
    Write-Host $logContent
    throw "scheduled task did not execute the installed action after registration"
}

Cleanup
