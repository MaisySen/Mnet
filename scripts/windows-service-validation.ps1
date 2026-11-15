# Native disposable Windows CI only. Exercises SCM and the local API, not
# Wintun/VPN networking. Never run against an existing mnetd installation.
param([Parameter(Mandatory=$true)][string]$BinaryPath)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'SCM validation requires an elevated disposable runner'
}
if (Get-Service -Name mnetd -ErrorAction SilentlyContinue) {
    throw 'Refusing to change an existing mnetd service'
}
$BinaryPath = (Resolve-Path $BinaryPath).Path
$state = Join-Path $env:RUNNER_TEMP ('mnet-scm-' + [guid]::NewGuid().ToString('N'))
$installed = $false
function Wait-ServiceState([string]$want) {
    $until = [DateTime]::UtcNow.AddSeconds(30)
    do {
        $service = Get-Service -Name mnetd
        if ($service.Status.ToString() -eq $want) { return }
        Start-Sleep -Milliseconds 200
    } while ([DateTime]::UtcNow -lt $until)
    throw "SCM did not reach $want"
}
function Assert-API {
    $tokenPath = Join-Path $state 'local-api-token'
    $token = (Get-Content -LiteralPath $tokenPath -Raw).Trim()
    $headers = @{ 'X-Mnet-Local-Token' = $token }
    $status = Invoke-RestMethod -Uri 'http://127.0.0.1:47373/localapi/status' -Headers $headers -TimeoutSec 3
    if ($status.status -ne 'down') { throw 'Unexpected daemon state in offline service fixture' }
    $response = Invoke-WebRequest -Uri 'http://127.0.0.1:47373/localapi/status' -SkipHttpErrorCheck -TimeoutSec 3
    if ($response.StatusCode -ne 401) { throw 'Unauthenticated local API access was not refused' }
}
function Assert-APIClosed {
    $until = [DateTime]::UtcNow.AddSeconds(5)
    do {
        try {
            $null = Invoke-WebRequest -Uri 'http://127.0.0.1:47373/localapi/status' -SkipHttpErrorCheck -TimeoutSec 2
        } catch { return }
        Start-Sleep -Milliseconds 100
    } while ([DateTime]::UtcNow -lt $until)
    throw 'Local API listener survived service stop'
}
try {
    & $BinaryPath -service install -state $state
    if ($LASTEXITCODE -ne 0) { throw 'Service install failed' }
    $installed = $true
    Start-Service -Name mnetd
    Wait-ServiceState 'Running'
    Assert-API
    Write-Host 'PASS SCM install/start and authenticated local API'

    Stop-Service -Name mnetd
    Wait-ServiceState 'Stopped'
    Assert-APIClosed
    Start-Service -Name mnetd
    Wait-ServiceState 'Running'
    Assert-API
    Write-Host 'PASS SCM clean stop/restart and listener teardown'

    $before = (Get-CimInstance Win32_Service -Filter "Name='mnetd'").ProcessId
    if ($before -eq 0) { throw 'Running service has no process' }
    Stop-Process -Id $before -Force
    $until = [DateTime]::UtcNow.AddSeconds(40)
    $recovered = $false
    do {
        Start-Sleep -Milliseconds 250
        $service = Get-CimInstance Win32_Service -Filter "Name='mnetd'"
        if ($service.State -eq 'Running' -and $service.ProcessId -ne 0 -and $service.ProcessId -ne $before) {
            $recovered = $true
            break
        }
    } while ([DateTime]::UtcNow -lt $until)
    if (-not $recovered) { throw 'SCM did not recover a forcibly terminated service' }
    Assert-API
    Write-Host 'PASS SCM crash recovery and reopened authenticated API'

    Stop-Service -Name mnetd
    Wait-ServiceState 'Stopped'
    & $BinaryPath -service remove
    if ($LASTEXITCODE -ne 0) { throw 'Service remove failed' }
    $installed = $false
    if (Get-Service -Name mnetd -ErrorAction SilentlyContinue) { throw 'Service survived removal' }
    Assert-APIClosed
    Write-Host 'PASS SCM removal'
} finally {
    if ($installed) {
        Stop-Service -Name mnetd -ErrorAction Continue
        & $BinaryPath -service remove
        if ($LASTEXITCODE -ne 0) { Write-Error 'SCM cleanup failed' }
    }
    if (Test-Path -LiteralPath $state) { Remove-Item -LiteralPath $state -Recurse -Force }
}
