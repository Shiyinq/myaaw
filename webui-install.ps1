# Install the Myaaw WebUI (dashboard) without the myaaw binary (Windows).
#
# Usage (PowerShell):
#   powershell -ExecutionPolicy Bypass -File .\webui-install.ps1
#   powershell -ExecutionPolicy Bypass -File .\webui-install.ps1 -Connect https://abc.ngrok-free.app
#
# Options:
#   -Version <tag>   Install a specific release tag (default: latest)
#   -Port <port>     WebUI port (default: 5050)
#   -ApiUrl <url>    Control-plane URL (default: http://localhost:9000)
#   -Connect <url>   Alias for -ApiUrl (remote control plane)
#   -Public          Public hosting: disable /setup and hide host hints

param(
    [string]$Version = "",
    [int]$Port = 5050,
    [string]$ApiUrl = "http://localhost:9000",
    [string]$Connect = "",
    [switch]$Public
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

if ($Connect) { $ApiUrl = $Connect }

$Repo = "Shiyinq/myaaw"
$WebUIDir = Join-Path $HOME ".myaaw\webui"

Write-Host "================================================="
Write-Host "Welcome to Myaaw WebUI Setup"
Write-Host "================================================="

if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
    Write-Host "Node.js 20+ is required but was not found in PATH." -ForegroundColor Red
    Write-Host "Install it from https://nodejs.org and re-run this script."
    exit 1
}
if (-not (Get-Command tar -ErrorAction SilentlyContinue)) {
    Write-Host "tar is required but was not found. Use Windows 10 (1803+) or install bsdtar." -ForegroundColor Red
    exit 1
}

if ($Version) {
    $ReleaseUrl = "https://api.github.com/repos/$Repo/releases/tags/$Version"
} else {
    $ReleaseUrl = "https://api.github.com/repos/$Repo/releases/latest"
}

Write-Host "Fetching the WebUI bundle..."
$Release = Invoke-RestMethod -Uri $ReleaseUrl -Headers @{ "User-Agent" = "myaaw-webui-install" }
$Asset = $Release.assets | Where-Object { $_.name -like "myaaw-webui-*.tar.gz" } | Select-Object -First 1

if (-not $Asset) {
    Write-Host "Could not find a WebUI bundle in the release." -ForegroundColor Red
    Write-Host "Check https://github.com/$Repo/releases"
    exit 1
}

Write-Host "Downloading $($Asset.name)..."
$Tmp = Join-Path $env:TEMP ("myaaw-webui-" + [System.Guid]::NewGuid().ToString() + ".tar.gz")
Invoke-WebRequest -Uri $Asset.browser_download_url -OutFile $Tmp

Write-Host "Installing to $WebUIDir\current..."
$Current = Join-Path $WebUIDir "current"
if (Test-Path $Current) { Remove-Item -Recurse -Force $Current }
New-Item -ItemType Directory -Force -Path $Current | Out-Null
tar -xzf $Tmp -C $Current
Remove-Item -Force $Tmp

$Config = @{
    port    = "$Port"
    api_url = $ApiUrl
    version = $(if ($Version) { $Version } else { "latest" })
    public  = [bool]$Public
} | ConvertTo-Json
Set-Content -Path (Join-Path $WebUIDir "config.json") -Value $Config -Encoding UTF8

Write-Host "WebUI installed at $Current" -ForegroundColor Green

$env:PORT = "$Port"
$env:HOST = "127.0.0.1"
$env:MYAAW_API_BASE_URL = $ApiUrl
if ($Public) { $env:MYAAW_WEBUI_PUBLIC = "1" }
$Log = Join-Path $WebUIDir "webui.log"
$ErrLog = Join-Path $WebUIDir "webui.err.log"

Start-Process -FilePath "node" -ArgumentList "build/index.js" -WorkingDirectory $Current `
    -RedirectStandardOutput $Log -RedirectStandardError $ErrLog -WindowStyle Hidden

Write-Host "WebUI started on http://127.0.0.1:$Port (control plane: $ApiUrl)" -ForegroundColor Green
Write-Host "Logs: $Log"
Write-Host ""
Write-Host "Open http://127.0.0.1:$Port in your browser."
if ($Public) {
    Write-Host "Public hosting: /setup and the login host hints are disabled."
}
