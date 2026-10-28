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
    $curlRoot = Join-Path ([IO.Path]::GetTempPath()) ('gasa-curl-' + [guid]::NewGuid().ToString('N')); New-Item -ItemType Directory $curlRoot | Out-Null
    try {
        $fakeCurl = Join-Path $curlRoot 'curl.ps1'
        @'
$headers = $null; $body = $null; $stderr = $null
for ($i = 0; $i -lt $args.Count; $i++) { switch ($args[$i]) { '-D' { $headers = $args[++$i] } '-o' { $body = $args[++$i] } '--stderr' { $stderr = $args[++$i] } } }
[IO.File]::WriteAllText($headers, "HTTP/1.1 200 OK`r`n`r`n"); if (-not $env:GASA_FAKE_MISSING_FILE) { [IO.File]::WriteAllText($body, 'ok') }; [IO.File]::WriteAllText($stderr, 'fake curl stderr')
Write-Output '200'; $global:LASTEXITCODE = [int]$env:GASA_FAKE_CURL_EXIT
'@ | Set-Content -LiteralPath $fakeCurl
        $env:GASA_FAKE_CURL_EXIT = '0'; $fakeOutput = Join-Path $curlRoot 'result.bin'
        Get-GitHubFile 'https://github.com/file' $fakeOutput -CurlPath $fakeCurl | Out-Null
        Report 'curl stderr file with exit 0 does not terminate PowerShell 5.1' ([IO.File]::ReadAllText($fakeOutput) -eq 'ok')
        $env:GASA_FAKE_MISSING_FILE = '1'; $missingOutput = Join-Path $curlRoot 'missing.bin'
        try { Get-GitHubFile 'https://github.com/file' $missingOutput -CurlPath $fakeCurl | Out-Null; Report 'curl success without file is rejected' $false }
        catch { Report 'curl success without file is rejected' ($_.Exception.Message -match 'did not create the downloaded file' -and -not (Test-Path $missingOutput)) }
        Remove-Item Env:GASA_FAKE_MISSING_FILE
        $env:GASA_FAKE_CURL_EXIT = '23'; $fakeFailure = Join-Path $curlRoot 'failed.bin'
    try { Get-GitHubFile 'https://github.com/file' $fakeFailure -CurlPath $fakeCurl | Out-Null; Report 'curl nonzero exit is rejected' $false }
        catch { Report 'curl nonzero exit is rejected' ($_.Exception.Message -match 'exit code 23' -and -not (Test-Path $fakeFailure)) }
        Add-Type -AssemblyName System.IO.Compression
        Add-Type -AssemblyName System.IO.Compression.FileSystem
        function New-TestZip([string]$Path,[string[]]$Names) {
            $zip = [IO.Compression.ZipFile]::Open($Path,[IO.Compression.ZipArchiveMode]::Create)
            try { foreach ($name in $Names) { $entry=$zip.CreateEntry($name); $stream=$entry.Open(); try { $bytes=if ($name -eq 'gasa.exe') { New-Object byte[] 2048 } else { [Text.Encoding]::UTF8.GetBytes('fixture') }; $stream.Write($bytes,0,$bytes.Length) } finally { $stream.Dispose() } } } finally { $zip.Dispose() }
        }
        $validZip=Join-Path $curlRoot 'allowed.zip'; $validOutput=Join-Path $curlRoot 'gasa.exe'
        New-TestZip $validZip @('gasa.exe','LICENSE','README.md')
        Get-ZipRootBinary $validZip $validOutput
        Report 'ZIP allows only regular LICENSE and README.md extras' ((Test-OrdinaryFile $validOutput) -and (Get-Item $validOutput).Length -eq 2048)
        $invalidZip=Join-Path $curlRoot 'unexpected.zip'; $invalidOutput=Join-Path $curlRoot 'invalid.exe'
        New-TestZip $invalidZip @('gasa.exe','script.sh')
        try { Get-ZipRootBinary $invalidZip $invalidOutput; Report 'ZIP rejects unexpected extra before extraction' $false }
        catch { Report 'ZIP rejects unexpected extra before extraction' ($_.Exception.Message -match 'unexpected entry' -and -not (Test-Path $invalidOutput)) }
    } finally { Remove-Item Env:GASA_FAKE_CURL_EXIT,Env:GASA_FAKE_MISSING_FILE -ErrorAction SilentlyContinue; Remove-Item $curlRoot -Recurse -Force -ErrorAction SilentlyContinue }
    $checksumName = 'gasa-test.zip'; $hashA = 'a' * 64; $hashB = 'b' * 64
    try { Get-ArchiveSha256 "$hashA  $checksumName`r`n$hashB  $checksumName`r`n" $checksumName | Out-Null; Report 'conflicting duplicate checksums abort' $false } catch { Report 'conflicting duplicate checksums abort' ($_.Exception.Message -match 'conflicting checksums') }
    try { Get-ArchiveSha256 "$hashB  $checksumName`n$hashA  $checksumName`n" $checksumName | Out-Null; Report 'reverse-order conflicting checksums abort' $false } catch { Report 'reverse-order conflicting checksums abort' ($_.Exception.Message -match 'conflicting checksums') }
    Report 'identical duplicate checksums accepted' ((Get-ArchiveSha256 "$hashA  $checksumName`r`n$hashA  $checksumName`r`n" $checksumName) -eq $hashA)
    Report 'uppercase checksum normalized' ((Get-ArchiveSha256 "$($hashA.ToUpperInvariant())  $checksumName`r`n" $checksumName) -ceq $hashA)
    Report 'BOM leading whitespace tab CRLF and optional star checksum accepted' ((Get-ArchiveSha256 (([string][char]0xFEFF) + " `t$hashA`t*$checksumName `r`n") $checksumName) -eq $hashA)
    try { $parsed = Get-ArchiveSha256 "$hashA *$checksumName`n$hashB  $checksumName.sig`n" $checksumName; Report 'exact archive checksum accepted and suffix ignored' ($parsed -ceq $hashA) } catch { Report 'exact archive checksum accepted and suffix ignored' $false }
    try { Get-ArchiveSha256 "$hashB  $checksumName.sbom`r`n" $checksumName | Out-Null; Report 'SBOM checksum suffix does not match archive' $false } catch { Report 'SBOM checksum suffix does not match archive' ($_.Exception.Message -match 'missing') }
    foreach ($separator in @([char]0x00a0,[char]0x3000)) {
        $candidate = "$hashA$separator$checksumName"
        try { Get-ArchiveSha256 $candidate $checksumName | Out-Null; Report "non-ASCII checksum separator $([int]$separator) rejected" $false }
        catch { Report "non-ASCII checksum separator $([int]$separator) rejected" ($_.Exception.Message -match 'missing') }
    }
    try { Get-ArchiveSha256 "$hashA  $checksumName`0`r`n" $checksumName | Out-Null; Report 'NUL in checksum filename does not match' $false }
    catch { Report 'NUL in checksum filename does not match' ($_.Exception.Message -match 'missing') }
    foreach ($row in [IO.File]::ReadAllLines((Join-Path $PSScriptRoot 'fixtures\checksum-parity.txt'))) {
        $fields = $row -split '\|',3; $caseName=$fields[0]; $expected=$fields[1]; $fixtureText=[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($fields[2])); $parityArchive='gasa_1.2.3_linux_amd64.tar.gz'
        try {
            $actual = Get-ArchiveSha256 $fixtureText $parityArchive
            Report "shared checksum fixture $caseName" ($expected -notin @('MISSING','CONFLICT') -and [string]::Equals($actual,$expected,[StringComparison]::Ordinal))
        } catch {
            $matched = ($expected -eq 'MISSING' -and $_.Exception.Message -match 'missing') -or ($expected -eq 'CONFLICT' -and $_.Exception.Message -match 'conflicting checksums')
            Report "shared checksum fixture $caseName" $matched $_.Exception.Message
        }
    }
    Report 'GitHub host matching is case insensitive' (Test-RedirectUri 'https://GitHub.com/releases/file' @('github.com'))
    $partial = Join-Path ([IO.Path]::GetTempPath()) ('gasa-limited-' + [guid]::NewGuid().ToString('N'))
    try { Copy-LimitedStream ([IO.MemoryStream]::new([byte[]](1,2,3,4,5,6))) $partial 5; Report 'limited zip copy rejects actual stream bytes and removes partial' $false }
    catch { Report 'limited zip copy rejects actual stream bytes and removes partial' (-not (Test-Path $partial)) }
    $recovery = Join-Path ([IO.Path]::GetTempPath()) ('gasa-recovery-' + [guid]::NewGuid().ToString('N')); New-Item -ItemType Directory $recovery | Out-Null
    try {
        [IO.File]::WriteAllText((Join-Path $recovery '.gasa-orphan.exe'), 'partial')
        $older = Join-Path $recovery 'gasa.exe.old.aaa'; $newer = Join-Path $recovery 'gasa.exe.old.zzz'
        [IO.File]::WriteAllText($older, 'older'); [IO.File]::WriteAllText($newer, 'newest')
        [IO.File]::SetLastWriteTimeUtc($older, [DateTime]::UtcNow.AddDays(-2)); [IO.File]::SetLastWriteTimeUtc($newer, [DateTime]::UtcNow.AddDays(-1))
        Recover-InstallState $recovery
        Report 'interrupted install restores the newest backup by mtime' (([IO.File]::ReadAllText((Join-Path $recovery 'gasa.exe')) -eq 'newest') -and -not (Test-Path (Join-Path $recovery '.gasa-orphan.exe')))
    } finally { Remove-Item $recovery -Recurse -Force -ErrorAction SilentlyContinue }
    $backupTest = Join-Path ([IO.Path]::GetTempPath()) ('gasa-backups-' + [guid]::NewGuid().ToString('N')); New-Item -ItemType Directory $backupTest | Out-Null
    try {
        $targetForBackup = Join-Path $backupTest 'gasa.exe'
        Report 'backup names are unique' ((New-UniqueBackupPath $targetForBackup) -ne (New-UniqueBackupPath $targetForBackup))
        $lockedBackup = Join-Path $backupTest 'gasa.exe.old.locked'; [IO.File]::WriteAllText($lockedBackup, 'backup'); [IO.File]::SetLastWriteTimeUtc($lockedBackup, [DateTime]::UtcNow.AddMinutes(-11))
        $lock = [IO.File]::Open($lockedBackup, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::None)
        try { Remove-OldBackups $backupTest; Report 'locked backup cleanup is tolerated and retained' (Test-Path $lockedBackup) } finally { $lock.Dispose() }
        Remove-OldBackups $backupTest
        Report 'unlocked old backup is cleaned' (-not (Test-Path $lockedBackup))
        $recentBackup = Join-Path $backupTest 'gasa.exe.old.concurrent'; [IO.File]::WriteAllText($recentBackup, 'backup')
        Remove-OldBackups $backupTest
        Report 'recent concurrent backup is retained' (Test-Path $recentBackup)
        $staleLock = Join-Path $backupTest '.gasa-install.lock'; [IO.File]::WriteAllText($staleLock, "99999999 $([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())`n")
        $stale = Enter-InstallLock $backupTest
        Exit-InstallLock $stale
        Report 'orphan install lock is discarded' (-not (Test-Path $staleLock))
        $held = Enter-InstallLock $backupTest
        try { try { Enter-InstallLock $backupTest | Out-Null; Report 'live install lock refuses second installer' $false } catch { Report 'live install lock refuses second installer and preserves lock' ($_.Exception.Message -match 'another installation is in progress' -and (Test-Path $staleLock)) } }
        finally { Exit-InstallLock $held }
    } finally { Remove-Item $backupTest -Recurse -Force -ErrorAction SilentlyContinue }
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
    $longPath = ('C:\keep;' * 400)
    $store = New-MemoryRegistry 'String' $longPath
    try { Update-UserPath $path $true $true $store; Report 'PATH over 2048 characters is allowed with warning' ($store.Value.Length -gt 2048 -and $store.Value.EndsWith($path)) } catch { Report 'PATH over 2048 characters is allowed with warning' $false $_.Exception.Message }
    $tooLong = 'x' * 32000; $store = New-MemoryRegistry 'String' $tooLong
    try { Update-UserPath $path $true $true $store; Report 'PATH near environment limit is blocked without mutation' $false } catch { Report 'PATH near environment limit is blocked without mutation' ($store.Value -eq $tooLong) }
    foreach ($badPath in @('C:\Users\x\gasa;evil\bin','C:\Users\x\gasa%evil\bin','C:\Users\x\gasa*evil\bin','C:\Users\x\gasa?evil\bin','C:\Users\x\gasa"evil\bin','C:\Users\x\gasa<evil\bin','C:\Users\x\gasa>evil\bin','C:\Users\x\gasa|evil\bin',"C:\Users\x\gasa`tevil\bin",("C:\Users\x\gasa" + [char]0x80 + 'evil\bin'),("C:\Users\x\gasa" + [char]0x200f + 'evil\bin'),("C:\Users\x\gasa" + [char]0x2029 + 'evil\bin'),("C:\Users\x\gasa" + [char]0x2069 + 'evil\bin'))) {
        $store = New-MemoryRegistry 'String' 'C:\keep'
        try { Update-UserPath $badPath $true $true $store; Report "direct PATH guard rejects $badPath" $false }
        catch { Report "direct PATH guard rejects $badPath" ($store.Value -eq 'C:\keep') }
    }
} catch { Report 'in-memory PATH adapter' $false $_.Exception.Message }

$temp = Join-Path ([IO.Path]::GetTempPath()) ('gasa-flow-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $temp | Out-Null
$serverFile = Join-Path $temp 'server.js'; $readyFile = Join-Path $temp 'ready.txt'; $stderrFile = Join-Path $temp 'node.err'
$serverSource = @'
const http=require('node:http'),fs=require('node:fs');
const ready=process.argv[2], log=process.argv[3];
const server=http.createServer((q,r)=>{
 fs.appendFileSync(log,q.url+'\n');
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
});server.listen(0,'127.0.0.1',()=>fs.writeFileSync(ready,String(server.address().port)));
'@
[IO.File]::WriteAllText($serverFile, $serverSource)
$accessLog = Join-Path $temp 'access.log'
$server = Start-Process -FilePath $node -ArgumentList @($serverFile, $readyFile, $accessLog) -PassThru -WindowStyle Hidden -RedirectStandardError $stderrFile
try {
    $until = [DateTime]::UtcNow.AddSeconds(10)
    while (-not (Test-Path $readyFile) -and [DateTime]::UtcNow -lt $until) { Start-Sleep -Milliseconds 50 }
    if (-not (Test-Path $readyFile)) { throw "local Node test server did not start: $(Get-Content $stderrFile -Raw -ErrorAction SilentlyContinue)" }
    $port = [IO.File]::ReadAllText($readyFile); $base = "http://127.0.0.1:$port"
    function DownloadCase([string]$Name, [string]$Route, [bool]$Expected, [int]$MaxBytes = 4096, [int]$MaxTime = 4, [switch]$Production, [switch]$FirstHopOnly) {
        $output = Join-Path $temp ([guid]::NewGuid().ToString('N') + '.bin')
        try {
            if ($Expected) { Get-GitHubFile "$base/$Route" $output -AllowedHosts @('127.0.0.1') -AllowHttp -MaxBytes $MaxBytes -MaxTimeSeconds $MaxTime | Out-Null; Report $Name ((Test-Path $output) -and [IO.File]::ReadAllText($output) -eq 'ok') }
            else {
                $parameters = @{ Uri = "$base/$Route"; OutFile = $output; AllowedHosts = @('127.0.0.1'); MaxBytes = $MaxBytes; MaxTimeSeconds = $MaxTime }
                if (-not $Production -and -not $FirstHopOnly) { $parameters.AllowHttp = $true }
                if ($FirstHopOnly) { $parameters.AllowHttpFirstHopOnly = $true }
                try { Get-GitHubFile @parameters | Out-Null; Report $Name $false 'download unexpectedly succeeded' }
                catch { Report $Name (-not (Test-Path $output)) $_.Exception.Message }
            }
        } catch { Report $Name $false $_.Exception.Message }
        finally { Remove-Item $output -Force -ErrorAction SilentlyContinue }
    }
    DownloadCase 'curl follows a valid chain of five redirects' 'r0' $true
    DownloadCase 'curl refuses six redirects and removes partial file' 's0' $false
    Remove-Item $accessLog -Force -ErrorAction SilentlyContinue
    DownloadCase 'final redirect host outside allowlist is refused before contacting it' 'external' $false
    Report 'blocked redirect host received no request' (-not (Select-String -Path $accessLog -SimpleMatch '/ok' -Quiet -ErrorAction SilentlyContinue))
    Remove-Item $accessLog -Force -ErrorAction SilentlyContinue
    DownloadCase 'production redirect to HTTP is refused after allowed test-only first hop' 'http' $false -FirstHopOnly
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
