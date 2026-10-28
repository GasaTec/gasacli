$ErrorActionPreference = 'Stop'
$installer = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\scripts\install.ps1'))
$node = (Get-Command node -CommandType Application).Source
$failed = $false
function Report([string]$Name, [bool]$Passed, [string]$Detail = '') {
    if ($Passed) { Write-Output "PASS: $Name" } else { Write-Output "FAIL: $Name $Detail"; $script:failed = $true }
}

$priorSourceOnly = $env:GASA_SOURCE_ONLY
try {
    $env:GASA_SOURCE_ONLY = '1'
    . $installer
    Report 'dot-source exposes download and path functions' ([bool](Get-Command Get-GitHubFile -ErrorAction SilentlyContinue) -and [bool](Get-Command Update-UserPath -ErrorAction SilentlyContinue))
    $source = [IO.File]::ReadAllText($installer)
    Invoke-Expression $source
    Report 'Invoke-Expression exposes installer functions' ([bool](Get-Command Get-GitHubFile -ErrorAction SilentlyContinue) -and [bool](Get-Command Invoke-GasaInstall -ErrorAction SilentlyContinue))
    $shellName = if ($PSVersionTable.PSEdition -eq 'Core') { 'pwsh.exe' } else { 'powershell.exe' }
    $shellPath = Join-Path $PSHOME $shellName
    $fileOutput = & $shellPath -NoProfile -ExecutionPolicy Bypass -File $installer 2>&1
    Report 'script also loads through -File in source-only mode' ($LASTEXITCODE -eq 0) ($fileOutput -join ' ')

    $env:GASA_UNINSTALL = '1'
    $env:GASA_INSTALL_DIR = Join-Path ([IO.Path]::GetTempPath()) ('gasa-missing-' + [guid]::NewGuid().ToString('N'))
    try { Invoke-GasaInstall; Report 'uninstall missing directory reports installer message' $false 'no error' }
    catch { Report 'uninstall missing directory reports installer message' ($_.Exception.Message -match 'No ordinary gasa.exe found') $_.Exception.Message }
    Remove-Item Env:GASA_UNINSTALL, Env:GASA_INSTALL_DIR -ErrorAction SilentlyContinue
} catch { Report 'PowerShell source loading' $false $_.Exception.Message }
finally { if ($null -eq $priorSourceOnly) { Remove-Item Env:GASA_SOURCE_ONLY -ErrorAction SilentlyContinue } else { $env:GASA_SOURCE_ONLY = $priorSourceOnly } }

# Registry operations are exercised against an in-memory adapter only; the real registry is never opened.
function New-MemoryRegistry([string]$Kind, [string]$Value) {
    $store = [pscustomobject]@{ Kind = $Kind; Value = $Value; Exists = $true; Deleted = $false }
    $store | Add-Member ScriptMethod GetValueNames { if ($this.Exists) { return @('Path') }; return @() }
    $store | Add-Member ScriptMethod GetValueKind { param($name) return [Enum]::Parse([Microsoft.Win32.RegistryValueKind], $this.Kind) }
    $store | Add-Member ScriptMethod GetValue { param($name,$default) if ($this.Exists) { return $this.Value }; return $default }
    $store | Add-Member ScriptMethod SetValue { param($name,$value,$kind) $this.Value = $value; $this.Kind = $kind.ToString(); $this.Exists = $true }
    $store | Add-Member ScriptMethod DeleteValue { param($name) $this.Exists = $false; $this.Deleted = $true }
    return $store
}
try {
    $path = Join-Path ([IO.Path]::GetTempPath()) 'gasa-path-test\bin'
    $store = New-MemoryRegistry 'String' "C:\\old;%USERPROFILE%;$(($path -replace '\\','/'))\..\bin\"
    Update-UserPath $path $true $true $store
    Report 'PATH canonicalized on write and REG_SZ retained with percent values' ($store.Kind -eq 'String' -and $store.Value.EndsWith($path, [StringComparison]::OrdinalIgnoreCase))
    $store = New-MemoryRegistry 'ExpandString' "$path;C:\\keep"
    Update-UserPath $path $false $false $store
    Report 'PATH removal compares normalized slash dot-dot and trailing slash' ($store.Value -eq 'C:\\keep')
    $store = New-MemoryRegistry 'String' $path
    Update-UserPath $path $false $false $store
    Report 'empty PATH is deleted' ($store.Deleted -and -not $store.Exists)
    $store = New-MemoryRegistry 'MultiString' $path
    try { Update-UserPath $path $true $true $store; Report 'MULTI_SZ PATH aborts without mutation' $false } catch { Report 'MULTI_SZ PATH aborts without mutation' ($store.Value -eq $path) }
} catch { Report 'in-memory PATH adapter' $false $_.Exception.Message }

$temp = Join-Path ([IO.Path]::GetTempPath()) ('gasa-flow-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
$serverFile = Join-Path $temp 'server.js'; $readyFile = Join-Path $temp 'ready.txt'; $stderrFile = Join-Path $temp 'node.err'
$serverSource = @'
const http=require('node:http'),fs=require('node:fs');
const ready=process.argv[2];
const server=http.createServer((q,r)=>{
 const n=Number((q.url.match(/^\/(?:r|s)(\d+)/)||[])[1]);
 if(q.url.startsWith('/r')&&n<4){r.writeHead(302,{location:'/r'+(n+1)});return r.end()}
 if(q.url==='/r4'){r.writeHead(302,{location:'/ok'});return r.end()}
 if(q.url.startsWith('/s')&&n<5){r.writeHead(302,{location:'/s'+(n+1)});return r.end()}
 if(q.url==='/s5'){r.writeHead(302,{location:'/ok'});return r.end()}
 if(q.url==='/external'){r.writeHead(302,{location:'http://127.0.0.2:'+server.address().port+'/ok'});return r.end()}
 if(q.url==='/http'){r.writeHead(302,{location:'http://127.0.0.1:'+server.address().port+'/ok'});return r.end()}
 if(q.url==='/port'){r.writeHead(302,{location:'https://github.com:8443/'});return r.end()}
 if(q.url==='/stall'){return}
 if(q.url==='/large'){r.writeHead(200,{'Content-Length':'1024'});return r.end(Buffer.alloc(1024))}
 if(q.url==='/ok'){r.writeHead(200);return r.end('ok')}
 r.writeHead(404);r.end();
});server.listen(0,'0.0.0.0',()=>fs.writeFileSync(ready,String(server.address().port)));
'@
[IO.File]::WriteAllText($serverFile, $serverSource)
$server = Start-Process -FilePath $node -ArgumentList @($serverFile, $readyFile) -PassThru -WindowStyle Hidden -RedirectStandardError $stderrFile
try {
    $until = [DateTime]::UtcNow.AddSeconds(10)
    while (-not (Test-Path $readyFile) -and [DateTime]::UtcNow -lt $until) { Start-Sleep -Milliseconds 50 }
    if (-not (Test-Path $readyFile)) { throw "local Node test server did not start: $(Get-Content $stderrFile -Raw -ErrorAction SilentlyContinue)" }
    $port = [IO.File]::ReadAllText($readyFile); $base = "http://127.0.0.1:$port"
    function DownloadCase([string]$Name, [string]$Route, [bool]$Expected, [int]$MaxBytes = 4096, [int]$MaxTime = 4, [switch]$Production) {
        $output = Join-Path $temp ([guid]::NewGuid().ToString('N') + '.bin')
        try {
            if ($Expected) { Get-GitHubFile "$base/$Route" $output -AllowedHosts @('127.0.0.1') -AllowHttp -MaxBytes $MaxBytes -MaxTimeSeconds $MaxTime | Out-Null; Report $Name ((Test-Path $output) -and [IO.File]::ReadAllText($output) -eq 'ok') }
            else {
                $parameters = @{ Uri = "$base/$Route"; OutFile = $output; AllowedHosts = @('127.0.0.1'); MaxBytes = $MaxBytes; MaxTimeSeconds = $MaxTime }
                if (-not $Production) { $parameters.AllowHttp = $true }
                try { Get-GitHubFile @parameters | Out-Null; Report $Name $false 'download unexpectedly succeeded' }
                catch { Report $Name (-not (Test-Path $output)) $_.Exception.Message }
            }
        } catch { Report $Name $false $_.Exception.Message }
        finally { Remove-Item $output -Force -ErrorAction SilentlyContinue }
    }
    DownloadCase 'curl follows a valid chain of five redirects' 'r0' $true
    DownloadCase 'curl refuses six redirects and removes partial file' 's0' $false
    DownloadCase 'final redirect host outside allowlist is refused' 'external' $false
    DownloadCase 'production redirect to HTTP is refused' 'http' $false -Production
    $portUri = "https://github.com:8443/"
    Report 'HTTPS URL on port 8443 is refused' (-not (Test-RedirectUri $portUri -AllowedHosts @('github.com')))
    DownloadCase 'body larger than limit is refused and removed' 'large' $false -MaxBytes 10
    $timer = [Diagnostics.Stopwatch]::StartNew(); DownloadCase 'stalled response times out and removes file' 'stall' $false -MaxTime 1; $timer.Stop()
    Report 'stall timeout is bounded' ($timer.Elapsed.TotalSeconds -lt 6)
    $bad = Join-Path $temp 'bad-archive.zip'; [IO.File]::WriteAllText($bad, 'archive'); $target = Join-Path $temp 'install\gasa.exe'
    $wrongHash = '0' * 64
    try { Assert-ArchiveChecksum $bad "$wrongHash  bad-archive.zip" 'bad-archive.zip' | Out-Null; New-Item -ItemType Directory (Split-Path $target) -Force | Out-Null; Copy-Item $bad $target } catch { }
    Report 'wrong checksum aborts before installation' (-not (Test-Path $target))
} catch { Report 'local download E2E setup' $false $_.Exception.Message }
finally { if ($server -and -not $server.HasExited) { Stop-Process -Id $server.Id -Force }; Remove-Item $temp -Recurse -Force -ErrorAction SilentlyContinue }

if ($failed) { exit 1 }
exit 0
