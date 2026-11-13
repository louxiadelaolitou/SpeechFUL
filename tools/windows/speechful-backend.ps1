param(
    [ValidateSet('Start','Stop','Status','Logs')][string]$Action = 'Status',
    [string]$Config
)
$ErrorActionPreference = 'Stop'

function Read-BackendConfig([string]$Path) {
    $c = Get-Content -LiteralPath $Path -Raw | ConvertFrom-Json
    if ($c.distro -notmatch '^[A-Za-z0-9._-]+$' -or $c.user -notmatch '^[a-z_][a-z0-9_-]*$') { throw 'Invalid WSL identity' }
    foreach ($unit in @($c.backendUnit,$c.proxyUnit)) {
        if ($unit -notmatch '^[a-zA-Z0-9_.@-]+\.service$') { throw 'Invalid service name' }
    }
    foreach ($port in @($c.communityPort,$c.verifierPort)) {
        if ([int]$port -lt 1 -or [int]$port -gt 65535) { throw 'Invalid loopback port' }
    }
    if ($c.communityPort -eq $c.verifierPort) { throw 'C/V ports must differ' }
    return $c
}

function Invoke-BackendWsl($Settings, [string[]]$Command, [switch]$Root) {
    $runUser = $Settings.user
    if ($Root) { $runUser = 'root' }
    $lines = @(& "$env:WINDIR\System32\wsl.exe" -d $Settings.distro -u $runUser -- @Command 2>&1)
    return [pscustomobject]@{ code=$LASTEXITCODE; lines=$lines }
}

function Get-LoopbackHealth([int]$Port) {
    $response = $null
    try {
        $request = [Net.HttpWebRequest]::Create("http://127.0.0.1:$Port/health/ready")
        $request.Proxy = $null
        $request.Timeout = 1500
        $response = $request.GetResponse()
        return [int]$response.StatusCode
    } catch {
        $cause = $_.Exception
        while ($cause.InnerException) { $cause = $cause.InnerException }
        if ($cause -is [Net.WebException]) { $response = $cause.Response }
        if ($response) { return [int]$response.StatusCode }
        return 0
    } finally {
        if ($response) { $response.Close() }
    }
}

function Get-BackendStatus($Settings) {
    $running = @(& "$env:WINDIR\System32\wsl.exe" --list --running --quiet 2>$null) -replace "`0",''
    if ($LASTEXITCODE -ne 0) { throw 'Cannot inspect WSL state' }
    $states = @('inactive','inactive')
    $community = 0; $verifier = 0
    if (@($running | ForEach-Object { $_.Trim() }) -contains $Settings.distro) {
        $units = @($Settings.backendUnit,$Settings.proxyUnit)
        for ($i=0; $i -lt 2; $i++) {
            $read = Invoke-BackendWsl $Settings @('systemctl','show',$units[$i],'-p','ActiveState','--value')
            if ($read.code -ne 0 -or $read.lines.Count -ne 1) { $states[$i]='unknown' }
            else { $states[$i]=[string]$read.lines[0] }
        }
        $community = Get-LoopbackHealth ([int]$Settings.communityPort)
        $verifier = Get-LoopbackHealth ([int]$Settings.verifierPort)
    }
    $ready = $states[0] -eq 'active' -and $states[1] -eq 'active' -and $community -eq 200 -and $verifier -eq 200
    $result = 'FAIL'; if ($ready) { $result='PASS' }
    return [pscustomobject]@{ result=$result; backend=$states[0]; proxy=$states[1]; communityHTTP=$community; verifierHTTP=$verifier }
}

function Write-BackendStatus($Status) {
    Write-Output ($Status | ConvertTo-Json -Compress)
    if ($Status.result -ne 'PASS') { Write-Output 'Backend unavailable. Use the backend logs tool. Frozen Gates require reviewed recovery; this tool never recovers them.' }
}

function Start-BackendKeeper($Settings,[string]$Record) {
    if (Test-Path -LiteralPath $Record) {
        $old = Get-Content -LiteralPath $Record -Raw | ConvertFrom-Json
        $owned = Get-Process -Id $old.pid -ErrorAction SilentlyContinue
        if ($owned -and $owned.ProcessName -eq 'wsl' -and $owned.StartTime.ToUniversalTime().ToString('o') -eq $old.startedUtc) { return }
    }
    $keeper = Start-Process -FilePath "$env:WINDIR\System32\wsl.exe" -ArgumentList @('-d',$Settings.distro,'-u',$Settings.user,'--','sleep','infinity') -WindowStyle Hidden -PassThru
    @{pid=$keeper.Id;startedUtc=$keeper.StartTime.ToUniversalTime().ToString('o')} | ConvertTo-Json | Set-Content -LiteralPath $Record -Encoding UTF8
}

function Invoke-BackendControl([string]$Mode,[string]$ConfigPath) {
    $settings = Read-BackendConfig $ConfigPath
    $keeperRecord = Join-Path (Split-Path -Parent ([IO.Path]::GetFullPath($ConfigPath))) 'backend-keeper.private.json'
    switch ($Mode) {
        'Start' {
            Start-BackendKeeper $settings $keeperRecord
            $reset = Invoke-BackendWsl $settings @('systemctl','reset-failed',$settings.backendUnit,$settings.proxyUnit) -Root
            if ($reset.code -ne 0) { throw 'Cannot reset service failure state' }
            $start = Invoke-BackendWsl $settings @('systemctl','start',$settings.backendUnit,$settings.proxyUnit) -Root
            $deadline = [DateTime]::UtcNow.AddSeconds(30)
            do {
                $state = Get-BackendStatus $settings
                if ($state.result -eq 'PASS') { Write-BackendStatus $state; return 0 }
                if ($state.backend -eq 'failed' -or $start.code -ne 0) { break }
                Start-Sleep -Seconds 1
            } while ([DateTime]::UtcNow -lt $deadline)
            Write-BackendStatus $state
            return 1
        }
        'Stop' {
            $stop = Invoke-BackendWsl $settings @('systemctl','stop',$settings.backendUnit,$settings.proxyUnit) -Root
            if ($stop.code -ne 0) { throw 'Backend stop failed' }
            if (Test-Path -LiteralPath $keeperRecord) {
                $old = Get-Content -LiteralPath $keeperRecord -Raw | ConvertFrom-Json
                $owned = Get-Process -Id $old.pid -ErrorAction SilentlyContinue
                if ($owned -and $owned.ProcessName -eq 'wsl' -and $owned.StartTime.ToUniversalTime().ToString('o') -eq $old.startedUtc) { Stop-Process -Id $owned.Id }
            }
            Write-Output 'PASS: backend and proxy stopped; tunnel processes were not changed.'
            return 0
        }
        'Status' {
            $state = Get-BackendStatus $settings
            Write-BackendStatus $state
            if ($state.result -eq 'PASS') { return 0 }; return 1
        }
        'Logs' {
            $logs = Invoke-BackendWsl $settings @('journalctl','-u',$settings.backendUnit,'-u',$settings.proxyUnit,'-n','80','--no-pager') -Root
            $logs.lines | ForEach-Object { Write-Host $_ }
            if ($logs.code -ne 0) { return $logs.code }
            if ($settings.diagnosticsScript -and $settings.runtimeRoot) {
                if ($settings.diagnosticsScript -notmatch '^/[A-Za-z0-9_./-]+$' -or $settings.runtimeRoot -notmatch '^/[A-Za-z0-9_./-]+$') { throw 'Invalid diagnostics path' }
                $diagnostics = Invoke-BackendWsl $settings @('python3','-B',$settings.diagnosticsScript,'report','--root',$settings.runtimeRoot) -Root
                $diagnostics.lines | ForEach-Object { Write-Host $_ }
                return $diagnostics.code
            }
            Write-Host 'UPLOAD_TELEMETRY=NOT_CONFIGURED (no upload diagnostics script in private config)'
            return 0
        }
    }
}

if ($MyInvocation.InvocationName -ne '.') {
    try {
        if (-not $Config) { throw 'A private config file is required' }
        # Keep status text off the pipeline carrying the numeric exit code.
        $output = @(Invoke-BackendControl $Action $Config)
        $code = [int]$output[-1]
        if ($output.Count -gt 1) { $output[0..($output.Count-2)] | ForEach-Object { Write-Output $_ } }
        exit $code
    } catch {
        Write-Output ('FAIL: ' + $_.Exception.Message)
        exit 1
    }
}
