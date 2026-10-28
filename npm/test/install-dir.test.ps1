$ErrorActionPreference = 'Stop'
$installer = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\scripts\install.ps1'))
$prior = $env:GASA_SOURCE_ONLY; $env:GASA_SOURCE_ONLY = '1'
try { . $installer } finally { if ($null -eq $prior) { Remove-Item Env:GASA_SOURCE_ONLY -ErrorAction SilentlyContinue } else { $env:GASA_SOURCE_ONLY = $prior } }
$tempRoot = Join-Path $env:TEMP ('gasa-policy-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$cases = @(
    @{ Name='default'; Path=(Join-Path $env:LOCALAPPDATA 'Programs\gasacli\bin'); Accept=$true },
    @{ Name='temp gasacli'; Path=(Join-Path $env:TEMP 'gasacli'); Accept=$true },
    @{ Name='space in path'; Path=(Join-Path $env:TEMP 'gasa space'); Accept=$true },
    @{ Name='temp gasa bin'; Path=(Join-Path $env:TEMP 'gasa-test\bin'); Accept=$true },
    @{ Name='home tools gasa'; Path=(Join-Path $HOME 'tools\gasa'); Accept=$true },
    @{ Name='drive root'; Path=[IO.Path]::GetPathRoot($env:SystemRoot); Accept=$false },
    @{ Name='extended question prefix'; Path="\\?\$($env:SystemDrive)\Users\$env:USERNAME"; Accept=$false },
    @{ Name='extended dot prefix'; Path="\\.\$($env:SystemDrive)\Users\$env:USERNAME"; Accept=$false },
    @{ Name='NT question prefix'; Path="\??\$($env:SystemDrive)\Users\$env:USERNAME"; Accept=$false },
    @{ Name='UNC'; Path='\\server\share\gasa'; Accept=$false },
    @{ Name='profile'; Path=$env:USERPROFILE; Accept=$false },
    @{ Name='profile trailing slash'; Path="$($env:USERPROFILE)\"; Accept=$false },
    @{ Name='profile forward slash'; Path=($env:USERPROFILE -replace '\\','/') + '/'; Accept=$false },
    @{ Name='system root'; Path=$env:SystemRoot; Accept=$false },
    @{ Name='program files'; Path=$env:ProgramFiles; Accept=$false },
    @{ Name='program files descendant'; Path=(Join-Path $env:ProgramFiles 'gasa'); Accept=$false },
    @{ Name='program data'; Path=$env:ProgramData; Accept=$false },
    @{ Name='program data descendant'; Path=(Join-Path $env:ProgramData 'gasa'); Accept=$false },
    @{ Name='public users'; Path=(Join-Path $env:PUBLIC 'gasa'); Accept=$false },
    @{ Name='system descendant'; Path=(Join-Path $env:SystemRoot 'gasa'); Accept=$false },
    @{ Name='relative path'; Path='relative\gasa'; Accept=$false },
    @{ Name='drive relative path'; Path='C:j-sim\gasa2'; Accept=$false },
    @{ Name='alternate drive relative path'; Path='D:foo\bin'; Accept=$false },
    @{ Name='ADS'; Path=(Join-Path $env:TEMP 'gasa:stream'); Accept=$false },
    @{ Name='ADS after drive'; Path='C:\Users\x\gasa:stream'; Accept=$false },
    @{ Name='semicolon rejected'; Path=(Join-Path $env:TEMP 'gasa;evil\bin'); Accept=$false },
    @{ Name='percent rejected'; Path=(Join-Path $env:TEMP 'gasa%TEMP%\bin'); Accept=$false },
    @{ Name='asterisk rejected'; Path=(Join-Path $env:TEMP 'gasa*'); Accept=$false },
    @{ Name='question rejected'; Path=(Join-Path $env:TEMP 'gasa?'); Accept=$false },
    @{ Name='quote rejected'; Path=(Join-Path $env:TEMP 'gasa"x'); Accept=$false },
    @{ Name='less-than rejected'; Path=(Join-Path $env:TEMP 'gasa<x'); Accept=$false },
    @{ Name='greater-than rejected'; Path=(Join-Path $env:TEMP 'gasa>x'); Accept=$false },
    @{ Name='pipe rejected'; Path=(Join-Path $env:TEMP 'gasa|x'); Accept=$false },
    @{ Name='control rejected'; Path=(Join-Path $env:TEMP "gasa`tx"); Accept=$false },
    @{ Name='DEL rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x7f + 'x')); Accept=$false },
    @{ Name='C1 U+0080 rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x80 + 'x')); Accept=$false },
    @{ Name='C1 U+009F rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x9f + 'x')); Accept=$false },
    @{ Name='direction mark U+200E rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x200e + 'x')); Accept=$false },
    @{ Name='line separator U+2028 rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x2028 + 'x')); Accept=$false },
    @{ Name='bidi override U+202E rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x202e + 'x')); Accept=$false },
    @{ Name='bidi isolate U+2066 rejected'; Path=(Join-Path $env:TEMP ("gasa" + [char]0x2066 + 'x')); Accept=$false },
    @{ Name='dot'; Path='.'; Accept=$false },
    @{ Name='dot dot'; Path='..'; Accept=$false }
)
$failed = $false
foreach ($version in @('v1.2.3','v1.2.3-alpha.1')) { if (Test-ReleaseVersion $version) { Write-Output "PASS: release version $version" } else { Write-Output "FAIL: release version $version was rejected"; $failed = $true } }
foreach ($version in @('v1.2.3-.','v1.2.3-..','v1.2.3-alpha.')) { if (-not (Test-ReleaseVersion $version)) { Write-Output "PASS: malformed release version $version rejected" } else { Write-Output "FAIL: malformed release version $version accepted"; $failed = $true } }
foreach ($case in $cases) {
    $reason = ''; $actual = Test-SafeInstallDirectory $case.Path @('bin','gasa*') 'reason'
    if ($actual -eq $case.Accept) { Write-Output "PASS: $($case.Name)" }
    else { Write-Output "FAIL: $($case.Name) expected=$($case.Accept) actual=$actual path=$($case.Path)"; $failed = $true }
}
foreach ($target in @($env:USERPROFILE, $env:LOCALAPPDATA, (Join-Path $env:SystemRoot 'System32'))) {
    $link = Join-Path $tempRoot ('gasa-junction-' + [guid]::NewGuid().ToString('N')); $created = $false
    try {
        & $env:ComSpec /d /c "mklink /J `"$link`" `"$target`"" | Out-Null
        if ($LASTEXITCODE -eq 0) {
            $created = $true
            if (-not (Test-SafeInstallDirectory $link @('bin','gasa*'))) { Write-Output "PASS: junction to $target" }
            else { Write-Output "FAIL: junction to $target was accepted"; $failed = $true }
        } else { Write-Output "FAIL: could not create test junction to $target"; $failed = $true }
    } finally { if ($created) { & $env:ComSpec /d /c "rmdir `"$link`"" | Out-Null } }
}
$savedLocalAppData = $env:LOCALAPPDATA
$fakeLocalAppData = Join-Path $tempRoot 'fake-local-appdata'
$defaultParent = Join-Path $fakeLocalAppData 'Programs\gasacli'
New-Item -ItemType Directory -Path $defaultParent -Force | Out-Null
$defaultLink = Join-Path $defaultParent 'bin'
try {
    $env:LOCALAPPDATA = $fakeLocalAppData
    & $env:ComSpec /d /c "mklink /J `"$defaultLink`" `"$env:USERPROFILE`"" | Out-Null
    if ($LASTEXITCODE -eq 0) {
        if (-not (Test-SafeInstallDirectory $defaultLink @('bin','gasa*'))) { Write-Output 'PASS: default destination junction rejected before default-path acceptance' }
        else { Write-Output 'FAIL: default destination junction was accepted'; $failed = $true }
        & $env:ComSpec /d /c "rmdir `"$defaultLink`"" | Out-Null
    } else { Write-Output 'FAIL: could not create default destination junction'; $failed = $true }
} finally { $env:LOCALAPPDATA = $savedLocalAppData }
Remove-Item -LiteralPath $tempRoot -Force -Recurse
if ($failed) { exit 1 }
exit 0
