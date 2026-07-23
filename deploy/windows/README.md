# Windows scheduled task deployment

This document uses ASD-STE100 Simplified Technical English.

Use Task Scheduler for native scheduled execution on Windows.

Windows deployments use NTFS ACLs for token-file privacy.
The registration helper disables inherited access on the token and its directory.
The helper gives explicit access to `SYSTEM` and local Administrators.
The scheduled task can then read the token.
`dns-update` also checks the file and directory ACLs at runtime.

The helper scripts in this directory:

- register a recurring scheduled task that runs as `SYSTEM`
- invoke `dns-update` with a config file path
- pass the Cloudflare token path and timeout through environment variables
- lock the token file and its dedicated credentials directory down to explicit
  NTFS access rules before task registration
- append combined stdout and stderr to a log file

Suggested installed layout:

- `C:\Program Files\dns-update\dns-update.exe`
- `C:\ProgramData\dns-update\config.json`
- `C:\ProgramData\dns-update\credentials\cloudflare.token`
- `C:\ProgramData\dns-update\dns-update.log`

## Registration parameters

Run the registration helper with Administrator permissions.

| Parameter | Default | Function |
| --- | --- | --- |
| `-TaskName` | `dns-update` | Set the scheduled-task name. |
| `-BinaryPath` | `C:\Program Files\dns-update\dns-update.exe` | Set the binary path. |
| `-ConfigPath` | `C:\ProgramData\dns-update\config.json` | Set the config path. |
| `-TokenPath` | `C:\ProgramData\dns-update\credentials\cloudflare.token` | Set the token path. |
| `-LogPath` | `C:\ProgramData\dns-update\dns-update.log` | Set the combined log path. |
| `-IntervalMinutes` | `5` | Set an interval from 1 through 1439 minutes. |
| `-Timeout` | `2m` | Set `DNS_UPDATE_TIMEOUT`. |
| `-ValidateConfig` | disabled | Run one config preflight before registration. |

The task runs as `SYSTEM` with the highest run level.
The task ignores a new start when an instance is active.
The task starts after it misses a scheduled start.
The task has a 10-minute execution limit.

`invoke-dns-update.ps1` is the task action wrapper.
Its path parameters are mandatory.
It passes the config, token, and timeout to `dns-update`.
It appends output and the exit code to the configured log.

| Wrapper parameter | Default | Function |
| --- | --- | --- |
| `-BinaryPath` | required | Set the binary path. |
| `-ConfigPath` | required | Set the config path. |
| `-TokenPath` | required | Set the token path. |
| `-LogPath` | required | Set the combined log path. |
| `-Timeout` | `2m` | Set `DNS_UPDATE_TIMEOUT`. |
| `-ValidateConfig` | disabled | Run config validation instead of reconciliation. |

Example:

```powershell
.\deploy\windows\register-scheduled-task.ps1 `
  -TaskName "dns-update" `
  -BinaryPath "C:\Program Files\dns-update\dns-update.exe" `
  -ConfigPath "C:\ProgramData\dns-update\config.json" `
  -TokenPath "C:\ProgramData\dns-update\credentials\cloudflare.token" `
  -LogPath "C:\ProgramData\dns-update\dns-update.log" `
  -IntervalMinutes 5
```

The registration helper uses the native `ScheduledTasks` PowerShell API and
replaces any existing task with the same name.

Pass `-ValidateConfig` to run an immediate preflight validation before the
helper registers the recurring task. The installed task still runs normal
reconciliation and does not keep `-ValidateConfig` in its action arguments.

For a fresh install:

```powershell
New-Item -ItemType Directory -Force -Path "C:\Program Files\dns-update" | Out-Null
New-Item -ItemType Directory -Force -Path "C:\ProgramData\dns-update" | Out-Null
New-Item -ItemType Directory -Force -Path "C:\ProgramData\dns-update\credentials" | Out-Null
Copy-Item .\dns-update.exe "C:\Program Files\dns-update\dns-update.exe" -Force
Copy-Item .\config.json "C:\ProgramData\dns-update\config.json" -Force
Copy-Item .\cloudflare.token "C:\ProgramData\dns-update\credentials\cloudflare.token" -Force

.\deploy\windows\register-scheduled-task.ps1 `
  -TaskName "dns-update" `
  -BinaryPath "C:\Program Files\dns-update\dns-update.exe" `
  -ConfigPath "C:\ProgramData\dns-update\config.json" `
  -TokenPath "C:\ProgramData\dns-update\credentials\cloudflare.token" `
  -LogPath "C:\ProgramData\dns-update\dns-update.log"
```

To inspect the registered task:

```powershell
Get-ScheduledTask -TaskName "dns-update"
Get-ScheduledTaskInfo -TaskName "dns-update"
Get-Content "C:\ProgramData\dns-update\dns-update.log" -Tail 50
```

To remove it:

```powershell
Unregister-ScheduledTask -TaskName "dns-update" -Confirm:$false
```
