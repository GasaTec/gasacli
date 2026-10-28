$script:GasaRepo = 'GasaTec/gasacli'
$script:GasaReleaseBase = "https://github.com/$($script:GasaRepo)/releases/download"
$script:GasaAllowedHosts = @('github.com', 'api.github.com', 'objects.githubusercontent.com', 'release-assets.githubusercontent.com')

function Get-CanonicalDirectory {
    param([string]$Directory)
    if ([string]::IsNullOrWhiteSpace($Directory) -or $Directory -match '^[A-Za-z]:(?![\\/])' -or $Directory -match '^[A-Za-z]:.*:') { return $null }
    if ($Directory -match '^(\\\\\?\\|\\\\\.\\|\\\?\?\\|//\?/)' -or $Directory.StartsWith('\\') -or $Directory.StartsWith('//')) { return $null }
    if (-not [IO.Path]::IsPathRooted($Directory)) { return $null }
    try {
        $value = [IO.Path]::GetFullPath($Directory)
        $root = [IO.Path]::GetPathRoot($value)
        if ($value.Length -gt $root.Length) { $value = $value.TrimEnd([char[]]@([char]92, [char]47)) }
        return $value
    } catch { return $null }
}

function Test-PathReparseComponents {
    param([string]$CanonicalPath)
    $root = [IO.Path]::GetPathRoot($CanonicalPath)
    $current = $root
    foreach ($part in ($CanonicalPath.Substring($root.Length) -split '[\\/]')) {
        if (-not $part) { continue }
        $current = Join-Path $current $part
        if (Test-Path -LiteralPath $current) {
            $item = Get-Item -LiteralPath $current -Force -ErrorAction SilentlyContinue
            if ($item -and ($item.Attributes -band [IO.FileAttributes]::ReparsePoint)) { return $current }
        }
    }
    return $null
}

function Test-SafeInstallDirectory {
    param([string]$Directory, [string[]]$AllowedLeaves = @('bin', 'gasa*'), [string]$ReasonVariable = '')
    $reason = 'path must be an absolute local path'
    $candidate = Get-CanonicalDirectory $Directory
    if (-not $candidate) { if ($ReasonVariable) { Set-Variable -Name $ReasonVariable -Value $reason -Scope 1 }; return $false }
    $reparse = Test-PathReparseComponents $candidate
    if ($reparse) { $reason = "reparse point is not allowed: $reparse"; if ($ReasonVariable) { Set-Variable $ReasonVariable $reason -Scope 1 }; return $false }
    $root = [IO.Path]::GetPathRoot($candidate)
    if ([string]::Equals($candidate, $root.TrimEnd([char]92), [StringComparison]::OrdinalIgnoreCase)) { $reason = 'drive roots are not allowed'; if ($ReasonVariable) { Set-Variable $ReasonVariable $reason -Scope 1 }; return $false }
    $leaf = [IO.Path]::GetFileName($candidate)
    if (-not (($AllowedLeaves | Where-Object { $leaf -like $_ }).Count)) { $reason = 'custom install directory must end in bin or a name beginning with gasa'; if ($ReasonVariable) { Set-Variable $ReasonVariable $reason -Scope 1 }; return $false }
    $protected = @($env:ProgramFiles, ${env:ProgramFiles(x86)}, $env:ProgramData, $env:PUBLIC, $env:SystemRoot)
    foreach ($directory in $protected) {
        if (-not $directory) { continue }
        $guard = Get-CanonicalDirectory $directory
        if ($guard -and ($candidate.Equals($guard, [StringComparison]::OrdinalIgnoreCase) -or $candidate.StartsWith($guard.TrimEnd([char]92) + '\', [StringComparison]::OrdinalIgnoreCase))) {
            $reason = "protected install location: $guard"; if ($ReasonVariable) { Set-Variable $ReasonVariable $reason -Scope 1 }; return $false
        }
    }
    return $true
}

function Test-RedirectUri {
    param([string]$Uri, [string[]]$AllowedHosts = $script:GasaAllowedHosts, [switch]$AllowHttp)
    try {
        $parsed = [Uri]$Uri
        $schemeOk = ($parsed.Scheme -ceq 'https') -or ($AllowHttp -and $parsed.Scheme -ceq 'http')
        $portOk = $parsed.IsDefaultPort -or $parsed.Port -eq 443 -or ($AllowHttp -and $parsed.Scheme -ceq 'http' -and $parsed.Port -gt 0)
        return $schemeOk -and $portOk -and ($AllowedHosts -icontains $parsed.Host.ToLowerInvariant())
    } catch { return $false }
}

function Test-ReleaseVersion {
    param([string]$Version)
    return $Version -cmatch '\Av[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?\z'
}

function Get-GitHubFile {
    param([string]$Uri, [string]$OutFile, [string[]]$AllowedHosts = $script:GasaAllowedHosts, [switch]$AllowHttp, [int]$MaxBytes = 209715200, [int]$MaxTimeSeconds = 120, [int]$ConnectTimeoutSeconds = 20)
    $curl = Join-Path $env:SystemRoot 'System32\curl.exe'
    if (-not (Test-Path -LiteralPath $curl -PathType Leaf)) { throw "curl.exe was not found at $curl; Windows 10 1803 or newer is required." }
    $protocol = if ($AllowHttp) { '=http,https' } else { '=https' }
    $protoRedirect = if ($AllowHttp) { '=http,https' } else { '=https' }
    try {
        $result = & $curl --fail --silent --show-error --location --proto $protocol --proto-redir $protoRedirect --max-redirs 5 --max-filesize $MaxBytes --max-time $MaxTimeSeconds --connect-timeout $ConnectTimeoutSeconds -H 'User-Agent: gasacli-installer' -o $OutFile -w '%{url_effective}' $Uri 2>&1
        if ($LASTEXITCODE -ne 0) { throw "curl.exe download failed (exit code $LASTEXITCODE): $($result -join ' ')" }
        $effective = [string]($result | Select-Object -Last 1)
        if (-not (Test-RedirectUri $effective -AllowedHosts $AllowedHosts -AllowHttp:$AllowHttp)) { throw "Untrusted final download URL: $effective" }
        return $effective
    } catch {
        Remove-Item -LiteralPath $OutFile -Force -ErrorAction SilentlyContinue
        throw
    }
}

function Get-ArchiveSha256 {
    param([string]$Text, [string]$ArchiveName)
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match '^([0-9a-fA-F]{64})\s+\*?(.+?)\s*$' -and $Matches[2] -ceq $ArchiveName) { return $Matches[1].ToLowerInvariant() }
    }
    throw "Archive missing from checksums.txt: $ArchiveName"
}

function Assert-ArchiveChecksum {
    param([string]$ArchivePath, [string]$ChecksumText, [string]$ArchiveName)
    $expected = Get-ArchiveSha256 $ChecksumText $ArchiveName
    $actual = (Get-FileHash -LiteralPath $ArchivePath -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -cne $expected) { throw "SHA-256 mismatch for $ArchiveName; nothing was installed." }
    return $expected
}

function Get-GasaArchitecture {
    $machine = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch -Regex ($machine) { '^AMD64$' { return 'amd64' } '^ARM64$' { return 'arm64' } default { throw "Unsupported Windows architecture: $machine" } }
}

function Test-OrdinaryFile {
    param([string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { return $false }
    $item = Get-Item -LiteralPath $Path -Force
    return -not (($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)
}

function Get-ZipRootBinary {
    param([string]$ArchivePath, [string]$Destination)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($ArchivePath)
    try {
        $entries = @($zip.Entries | Where-Object { $_.FullName -ceq 'gasa.exe' })
        if ($entries.Count -ne 1) { throw 'The release archive must contain exactly one gasa.exe at its root.' }
        $entry = $entries[0]
        $unixType = ($entry.ExternalAttributes -shr 16) -band 0xF000
        if ($entry.FullName.Contains('..') -or $entry.FullName.Contains('/') -or $entry.FullName.Contains('\\') -or [IO.Path]::IsPathRooted($entry.FullName) -or ($unixType -ne 0 -and $unixType -ne 0x8000)) { throw 'Unsafe or non-regular path in release archive.' }
        if ($entry.Length -gt 104857600) { throw 'Extracted executable exceeds 100 MiB.' }
        $input = $entry.Open()
        try { $output = [IO.File]::Open($Destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None); try { $input.CopyTo($output) } finally { $output.Dispose() } } finally { $input.Dispose() }
    } finally { $zip.Dispose() }
    if (-not (Test-OrdinaryFile $Destination) -or (Get-Item -LiteralPath $Destination).Length -lt 1024) { throw 'Downloaded executable is missing, not a regular file, or too small.' }
}

function Test-BinaryVersion {
    param([string]$Path)
    & $Path --version | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "gasa --version failed for $Path" }
}

function Normalize-PathEntry {
    param([string]$Value)
    $value = $Value.Trim().Replace('/', '\')
    try { return [IO.Path]::GetFullPath($value).TrimEnd([char]92) } catch { return $value.TrimEnd([char]92) }
}

function Update-UserPath {
    param([string]$Directory, [bool]$Add, [bool]$ValidateLength = $true, $RegistryStore = $null)
    $canonical = Get-CanonicalDirectory $Directory
    if (-not $canonical) { throw 'PATH directory must be an absolute canonical path.' }
    if ($null -eq $RegistryStore) {
        $RegistryStore = [pscustomobject]@{ Key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true) }
        if (-not $RegistryStore.Key) { $RegistryStore.Key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment') }
        $RegistryStore | Add-Member ScriptMethod GetValueNames { $this.Key.GetValueNames() }
        $RegistryStore | Add-Member ScriptMethod GetValueKind { param($name) $this.Key.GetValueKind($name) }
        $RegistryStore | Add-Member ScriptMethod GetValue { param($name,$default) $this.Key.GetValue($name,$default,[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames) }
        $RegistryStore | Add-Member ScriptMethod SetValue { param($name,$value,$kind) $this.Key.SetValue($name,$value,$kind) }
        $RegistryStore | Add-Member ScriptMethod DeleteValue { param($name) $this.Key.DeleteValue($name,$false) }
    }
    try {
        $names = @($RegistryStore.GetValueNames())
        $exists = $names -contains 'Path'
        $kind = if ($exists) { $RegistryStore.GetValueKind('Path') } else { [Microsoft.Win32.RegistryValueKind]::ExpandString }
        if ($kind -notin @([Microsoft.Win32.RegistryValueKind]::String, [Microsoft.Win32.RegistryValueKind]::ExpandString)) { throw "Unsupported user PATH registry type: $kind; PATH was not changed." }
        $oldPath = if ($exists) { [string]$RegistryStore.GetValue('Path', '') } else { '' }
        if ($kind -eq [Microsoft.Win32.RegistryValueKind]::MultiString) { throw 'Unsupported user PATH registry type: MultiString; PATH was not changed.' }
        $parts = @($oldPath -split ';' | Where-Object { $_.Trim() })
        $filtered = @($parts | Where-Object { -not [string]::Equals((Normalize-PathEntry $_), (Normalize-PathEntry $canonical), [StringComparison]::OrdinalIgnoreCase) })
        if ($Add) { $filtered += $canonical }
        $newPath = $filtered -join ';'
        if ($Add -and $ValidateLength -and $newPath.Length -gt 2048) { throw 'User PATH would exceed 2048 characters; it was not changed.' }
        if (-not $newPath) { $RegistryStore.DeleteValue('Path') }
        else { $RegistryStore.SetValue('Path', $newPath, $kind) }
    } finally { if ($RegistryStore.PSObject.Properties['Key'] -and $RegistryStore.Key) { $RegistryStore.Key.Dispose() } }
    $current = @($env:Path -split ';' | Where-Object { $_ -and -not [string]::Equals((Normalize-PathEntry $_), (Normalize-PathEntry $canonical), [StringComparison]::OrdinalIgnoreCase) })
    if ($Add) { $current += $canonical }
    $env:Path = $current -join ';'
    if ($env:OS -eq 'Windows_NT') {
        try {
            if (-not ('GasaPathBroadcast' -as [type])) { Add-Type -Namespace Native -Name GasaPathBroadcast -MemberDefinition '[System.Runtime.InteropServices.DllImport("user32.dll", CharSet=CharSet.Auto, SetLastError=true)] public static extern IntPtr SendMessageTimeout(IntPtr hWnd, uint Msg, UIntPtr wParam, string lParam, uint flags, uint timeout, out UIntPtr result);' }
            $result = [UIntPtr]::Zero; [void][Native.GasaPathBroadcast]::SendMessageTimeout([IntPtr]0xffff, 0x001A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result)
        } catch { }
    }
}

function Get-ResolvedVersion {
    $version = $env:GASA_VERSION
    if (-not $version) {
        $temp = Join-Path ([IO.Path]::GetTempPath()) ('gasa-release-' + [guid]::NewGuid().ToString('N'))
        Get-GitHubFile "https://api.github.com/repos/$($script:GasaRepo)/releases?per_page=1" $temp | Out-Null
        try { $releases = [IO.File]::ReadAllText($temp) | ConvertFrom-Json } finally { Remove-Item -LiteralPath $temp -Force -ErrorAction SilentlyContinue }
        if (-not $releases -or -not $releases[0].tag_name) { throw 'No GitHub release was found.' }
        $version = $releases[0].tag_name
    }
    if (-not (Test-ReleaseVersion $version)) { throw "Unsupported release version: $version" }
    return $version
}

function Invoke-GasaInstall {
    $ErrorActionPreference = 'Stop'
    $installDir = if ($env:GASA_INSTALL_DIR) { $env:GASA_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA 'Programs\gasacli\bin' }
    $reason = ''
    if (-not (Test-SafeInstallDirectory $installDir @('bin','gasa*') 'reason')) { throw "Refusing install directory '$installDir': $reason" }
    $installDir = Get-CanonicalDirectory $installDir
    $selectedMethod = if ($env:GASA_METHOD) { $env:GASA_METHOD } else { 'binary' }
    if ($selectedMethod -notin @('binary','npm')) { throw 'GASA_METHOD must be binary or npm.' }
    if ($env:GASA_UNINSTALL -eq '1') {
        $binary = Join-Path $installDir 'gasa.exe'
        if (-not (Test-OrdinaryFile $binary)) { throw "No ordinary gasa.exe found in $installDir; nothing was uninstalled." }
        Remove-Item -LiteralPath $binary -Force
        Remove-Item -LiteralPath (Join-Path $installDir 'gasa.exe.old') -Force -ErrorAction SilentlyContinue
        if (@(Get-ChildItem -LiteralPath $installDir -Force -ErrorAction SilentlyContinue).Count -eq 0) { Remove-Item -LiteralPath $installDir -Force }
        Update-UserPath $installDir $false $false
        Write-Host 'Installation removed; data in %APPDATA%\gasacli was preserved.'
        return
    }
    $version = Get-ResolvedVersion
    if ($selectedMethod -eq 'npm') {
        $npm = Get-Command npm -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if (-not $npm) { throw 'npm.cmd was not found. Use the direct binary installer.' }
        $npmPath = [IO.Path]::ChangeExtension($npm.Source, '.cmd')
        & $npmPath install --global "gasacli@$($version.Substring(1))"
        if ($LASTEXITCODE -ne 0) { throw 'npm installation failed.' }
        return
    }
    $arch = Get-GasaArchitecture
    $archiveName = 'gasa_{0}_windows_{1}.zip' -f $version.Substring(1), $arch
    $work = Join-Path ([IO.Path]::GetTempPath()) ('gasacli-' + [guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $work | Out-Null
    try {
        $checksums = Join-Path $work 'checksums.txt'; $archive = Join-Path $work $archiveName
        Get-GitHubFile "$($script:GasaReleaseBase)/$version/checksums.txt" $checksums | Out-Null
        Get-GitHubFile "$($script:GasaReleaseBase)/$version/$archiveName" $archive | Out-Null
        Assert-ArchiveChecksum $archive ([IO.File]::ReadAllText($checksums)) $archiveName | Out-Null
        $source = Join-Path $work 'gasa.exe'; Get-ZipRootBinary $archive $source; Test-BinaryVersion $source
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
        $target = Join-Path $installDir 'gasa.exe'; $old = "$target.old"; $staged = Join-Path $installDir ('.gasa-' + [guid]::NewGuid().ToString('N') + '.exe')
        $hadTarget = Test-Path -LiteralPath $target; $moved = $false
        try {
            [IO.File]::Copy($source, $staged, $false)
            if ($hadTarget) { Move-Item -LiteralPath $target -Destination $old -Force; $moved = $true }
            Move-Item -LiteralPath $staged -Destination $target -Force; Test-BinaryVersion $target
            Update-UserPath $installDir $true
        } catch {
            if (-not $hadTarget -or $moved) { Remove-Item -LiteralPath $target -Force -ErrorAction SilentlyContinue }
            if ($moved -and (Test-Path -LiteralPath $old)) { Move-Item -LiteralPath $old -Destination $target -Force }
            throw
        } finally { Remove-Item -LiteralPath $staged -Force -ErrorAction SilentlyContinue }
        Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue
    } finally { Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue }
    Write-Host 'Installed. Open a new terminal to refresh PATH.'
}

if ($env:GASA_SOURCE_ONLY -ne '1') {
    try { Invoke-GasaInstall } catch { Write-Error $_.Exception.Message; exit 1 }
}
