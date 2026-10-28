$ErrorActionPreference = 'Stop'
$installer = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\..\scripts\install.ps1'))
$prior = $env:GASA_SOURCE_ONLY; $env:GASA_SOURCE_ONLY = '1'
try { . $installer } finally { if ($null -eq $prior) { Remove-Item Env:GASA_SOURCE_ONLY -ErrorAction SilentlyContinue } else { $env:GASA_SOURCE_ONLY = $prior } }
$tempRoot = Join-Path $env:TEMP ('gasa-policy-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempRoot | Out-Null
$cases = @(
    @{ Name='default'; Path=(Join-Path $env:LOCALAPPDATA 'Programs\gasacli\bin'); Accept=$true },
    @{ Name='temp gasacli'; Path=(Join-Path $env:TEMP 'gasacli'); Accept=$true },
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
    @{ Name='dot'; Path='.'; Accept=$false },
    @{ Name='dot dot'; Path='..'; Accept=$false }
)
$failed = $false
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
