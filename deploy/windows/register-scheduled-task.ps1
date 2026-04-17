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
$tokenDir = Split-Path -Parent $tokenPath
$logPath = [System.IO.Path]::GetFullPath($LogPath)
$powerShellPath = Join-Path $env:SystemRoot "System32\WindowsPowerShell\v1.0\powershell.exe"
$workingDirectory = Split-Path -Parent $binaryPath

$logDir = Split-Path -Parent $logPath
if ($logDir) {
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}

function Get-RequiredTokenPrincipals {
    $currentUser = [System.Security.Principal.WindowsIdentity]::GetCurrent().User
    if ($null -eq $currentUser) {
        throw "Unable to resolve the current Windows identity."
    }

    return @(
        [System.Security.Principal.SecurityIdentifier]::new([System.Security.Principal.WellKnownSidType]::LocalSystemSid, $null),
        [System.Security.Principal.SecurityIdentifier]::new([System.Security.Principal.WellKnownSidType]::BuiltinAdministratorsSid, $null),
        $currentUser
    )
}

function Protect-PathAcl {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $acl = Get-Acl -LiteralPath $Path
    $acl.SetAccessRuleProtection($true, $false)
    foreach ($rule in @($acl.Access)) {
        $null = $acl.RemoveAccessRuleSpecific($rule)
    }

    foreach ($principal in (Get-RequiredTokenPrincipals)) {
        $rule = [System.Security.AccessControl.FileSystemAccessRule]::new(
            $principal,
            [System.Security.AccessControl.FileSystemRights]::FullControl,
            [System.Security.AccessControl.InheritanceFlags]::None,
            [System.Security.AccessControl.PropagationFlags]::None,
            [System.Security.AccessControl.AccessControlType]::Allow
        )
        $null = $acl.AddAccessRule($rule)
    }

    Set-Acl -LiteralPath $Path -AclObject $acl
}

if ($tokenDir) {
    Protect-PathAcl -Path $tokenDir
}
Protect-PathAcl -Path $tokenPath

function New-TaskArguments {
    param(
        [switch]$ValidateOnly
    )

    $arguments = @(
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
    if ($ValidateOnly) {
        $arguments += "-ValidateConfig"
    }

    return $arguments
}

function Wait-TaskCompletion {
    param(
        [Parameter(Mandatory = $true)]
        [string]$TaskName,
        [Parameter(Mandatory = $true)]
        [datetime]$PreviousLastRunTime,
        [timespan]$Timeout = (New-TimeSpan -Minutes 2)
    )

    $deadline = (Get-Date).Add($Timeout)
    while ((Get-Date) -lt $deadline) {
        $task = Get-ScheduledTask -TaskName $TaskName
        $info = Get-ScheduledTaskInfo -TaskName $TaskName
        if ($info.LastRunTime -ne $PreviousLastRunTime -and $task.State -notin @("Queued", "Running")) {
            return $info.LastTaskResult
        }

        Start-Sleep -Seconds 2
    }

    throw "Task '$TaskName' did not finish within $($Timeout.TotalSeconds) seconds."
}

$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet `
    -ExecutionTimeLimit (New-TimeSpan -Minutes 10) `
    -MultipleInstances IgnoreNew `
    -StartWhenAvailable

if ($ValidateConfig) {
    $validationTaskName = "$TaskName-validate-config"
    $validationAction = New-ScheduledTaskAction `
        -Execute $powerShellPath `
        -Argument ((New-TaskArguments -ValidateOnly) -join " ") `
        -WorkingDirectory $workingDirectory
    $validationTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(5)

    try {
        Unregister-ScheduledTask -TaskName $validationTaskName -Confirm:$false -ErrorAction Stop | Out-Null
    } catch {
    }

    Register-ScheduledTask `
        -TaskName $validationTaskName `
        -Action $validationAction `
        -Trigger $validationTrigger `
        -Principal $principal `
        -Settings $settings `
        -Force | Out-Null

    $validationResult = $null
    try {
        $validationInfo = Get-ScheduledTaskInfo -TaskName $validationTaskName
        Start-ScheduledTask -TaskName $validationTaskName
        $validationResult = Wait-TaskCompletion `
            -TaskName $validationTaskName `
            -PreviousLastRunTime $validationInfo.LastRunTime
    } finally {
        try {
            Unregister-ScheduledTask -TaskName $validationTaskName -Confirm:$false -ErrorAction Stop | Out-Null
        } catch {
        }
    }

    if ($validationResult -ne 0) {
        throw "ValidateConfig preflight task failed with exit code $validationResult."
    }

    Remove-Item -LiteralPath $logPath -Force -ErrorAction SilentlyContinue
}

$taskArguments = New-TaskArguments

$action = New-ScheduledTaskAction `
    -Execute $powerShellPath `
    -Argument ($taskArguments -join " ") `
    -WorkingDirectory $workingDirectory
$trigger = New-ScheduledTaskTrigger `
    -Once `
    -At (Get-Date).AddMinutes(1) `
    -RepetitionInterval (New-TimeSpan -Minutes $IntervalMinutes) `
    -RepetitionDuration (New-TimeSpan -Days 3650)

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
