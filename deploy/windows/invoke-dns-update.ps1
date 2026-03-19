param(
    [Parameter(Mandatory = $true)]
    [string]$BinaryPath,
    [Parameter(Mandatory = $true)]
    [string]$ConfigPath,
    [Parameter(Mandatory = $true)]
    [string]$TokenPath,
    [Parameter(Mandatory = $true)]
    [string]$LogPath,
    [string]$Timeout = "2m",
    [switch]$ValidateConfig
)

$ErrorActionPreference = "Stop"

$logDir = Split-Path -Parent $LogPath
if ($logDir) {
    New-Item -ItemType Directory -Path $logDir -Force | Out-Null
}

$env:DNS_UPDATE_PROVIDER_CLOUDFLARE_API_TOKEN_FILE = $TokenPath
$env:DNS_UPDATE_TIMEOUT = $Timeout

$arguments = @("-config", $ConfigPath)
if ($ValidateConfig) {
    $arguments = @("-validate-config") + $arguments
}

$output = & $BinaryPath @arguments 2>&1
$output | Out-File -FilePath $LogPath -Append -Encoding utf8

$exitCode = $LASTEXITCODE
if ($null -eq $exitCode) {
    $exitCode = 0
}

exit $exitCode
