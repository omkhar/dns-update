# Windows scheduled task deployment

Use Task Scheduler for native scheduled execution on Windows.

The helper scripts in this directory:

- register a recurring scheduled task that runs as `SYSTEM`
- invoke `dns-update` with a config file path
- pass the Cloudflare token path and timeout through environment variables
- append combined stdout and stderr to a log file

Suggested installed layout:

- `C:\Program Files\dns-update\dns-update.exe`
- `C:\ProgramData\dns-update\config.json`
- `C:\ProgramData\dns-update\cloudflare.token`
- `C:\ProgramData\dns-update\dns-update.log`

Example:

```powershell
.\deploy\windows\register-scheduled-task.ps1 `
  -TaskName "dns-update" `
  -BinaryPath "C:\Program Files\dns-update\dns-update.exe" `
  -ConfigPath "C:\ProgramData\dns-update\config.json" `
  -TokenPath "C:\ProgramData\dns-update\cloudflare.token" `
  -LogPath "C:\ProgramData\dns-update\dns-update.log" `
  -IntervalMinutes 5
```

The registration helper uses the native `ScheduledTasks` PowerShell API and
replaces any existing task with the same name.

For a fresh install:

```powershell
New-Item -ItemType Directory -Force -Path "C:\Program Files\dns-update" | Out-Null
New-Item -ItemType Directory -Force -Path "C:\ProgramData\dns-update" | Out-Null
Copy-Item .\dns-update.exe "C:\Program Files\dns-update\dns-update.exe" -Force
Copy-Item .\config.json "C:\ProgramData\dns-update\config.json" -Force
Copy-Item .\cloudflare.token "C:\ProgramData\dns-update\cloudflare.token" -Force

.\deploy\windows\register-scheduled-task.ps1 `
  -TaskName "dns-update" `
  -BinaryPath "C:\Program Files\dns-update\dns-update.exe" `
  -ConfigPath "C:\ProgramData\dns-update\config.json" `
  -TokenPath "C:\ProgramData\dns-update\cloudflare.token" `
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
