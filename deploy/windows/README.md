# Windows scheduled task deployment

Use Task Scheduler for native scheduled execution on Windows.

The helper scripts in this directory:

- register a recurring scheduled task that runs as `SYSTEM`
- invoke `dns-update` with a config file path
- pass the Cloudflare token path and timeout through environment variables
- append combined stdout and stderr to a log file

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
