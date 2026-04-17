# Windows scheduled task deployment

Use Task Scheduler for native scheduled execution on Windows.

Windows deployments rely on NTFS ACLs for token-file privacy. The registration
helper now disables inherited access on the token file and its dedicated
credentials directory, then replaces it with explicit rules for `SYSTEM` and
local Administrators so the scheduled task can read the token without leaving
it broadly readable. `dns-update` also validates that the token file and its
parent directory do not grant risky access to other Windows users at runtime.

The helper scripts in this directory:

- register a recurring scheduled task that runs as `SYSTEM`
- invoke `dns-update` with a config file path
- pass the Cloudflare token path and timeout through environment variables
- lock the token file and its dedicated credentials directory down to explicit
  NTFS access rules before registering the task
- append combined stdout and stderr to a log file

Suggested installed layout:

- `C:\Program Files\dns-update\dns-update.exe`
- `C:\ProgramData\dns-update\config.json`
- `C:\ProgramData\dns-update\credentials\cloudflare.token`
- `C:\ProgramData\dns-update\dns-update.log`

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
recurring task is registered. The installed task still runs normal
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
