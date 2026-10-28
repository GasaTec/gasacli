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
    $hasControl = $false
    foreach ($character in $Directory.ToCharArray()) { $code = [int]$character; if ($code -lt 32 -or ($code -ge 0x7f -and $code -le 0x9f) -or $code -in @(0x200e,0x200f,0x2028,0x2029) -or ($code -ge 0x202a -and $code -le 0x202e) -or ($code -ge 0x2066 -and $code -le 0x2069)) { $hasControl = $true; break } }
    if ($Directory -match '[;%*?"<>|]' -or $hasControl) { $reason = 'install directory contains a prohibited character'; if ($ReasonVariable) { Set-Variable -Name $ReasonVariable -Value $reason -Scope 1 }; return $false }
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
    return $Version -cmatch '\Av[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?\z'
}

function Get-GitHubFile {
    param([string]$Uri, [string]$OutFile, [string[]]$AllowedHosts = $script:GasaAllowedHosts, [switch]$AllowHttp, [switch]$AllowHttpFirstHopOnly, [int]$MaxBytes = 209715200, [int]$MaxTimeSeconds = 120, [int]$ConnectTimeoutSeconds = 20, [string]$CurlPath = '')
    $priorErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $curl = if ($CurlPath) { $CurlPath } else { Join-Path $env:SystemRoot 'System32\curl.exe' }
    if (-not (Test-Path -LiteralPath $curl -PathType Leaf)) { $ErrorActionPreference = $priorErrorActionPreference; throw "curl.exe was not found at $curl; Windows 10 1803 or newer is required." }
    $stderrFile = [IO.Path]::GetTempFileName()
    try {
        $current = $Uri
        for ($hop = 0; $hop -le 5; $hop++) {
            $firstHopHttp = ($AllowHttpFirstHopOnly -and $hop -eq 0) -or $AllowHttp
            if (-not (Test-RedirectUri $current -AllowedHosts $AllowedHosts -AllowHttp:($AllowHttp -or ($AllowHttpFirstHopOnly -and $hop -eq 0)))) { throw "Untrusted download URL: $current" }
            $headers = "$OutFile.headers"; $body = "$OutFile.part"
            Remove-Item $headers,$body -Force -ErrorAction SilentlyContinue
            $proto = if ($firstHopHttp) { '=http,https' } else { '=https' }
            $result = & $curl --silent --show-error --proto $proto --max-filesize $MaxBytes --max-time $MaxTimeSeconds --connect-timeout $ConnectTimeoutSeconds -D $headers -o $body -w '%{http_code}' -H 'User-Agent: gasacli-installer' --stderr $stderrFile $current
            $curlExitCode = $LASTEXITCODE
            if ($curlExitCode -ne 0) { $curlError = [IO.File]::ReadAllText($stderrFile); throw "curl.exe download failed (exit code $curlExitCode): $curlError" }
            $status = [int]([string]($result | Select-Object -Last 1))
            if ($status -notin @(301,302,303,307,308)) {
                if ($status -ne 200) { throw "Download failed with HTTP $status." }
                if (-not (Test-Path -LiteralPath $body -PathType Leaf)) { throw 'curl.exe reported success but did not create the downloaded file.' }
                Move-Item -LiteralPath $body -Destination $OutFile -Force -ErrorAction Stop
                if (-not (Test-Path -LiteralPath $OutFile -PathType Leaf)) { throw 'Downloaded file is missing after move.' }
                Remove-Item $headers -Force -ErrorAction SilentlyContinue; return $current
            }
            if ($hop -eq 5) { throw 'Too many redirects while downloading release asset.' }
            $location = Get-Content -LiteralPath $headers | Where-Object { $_ -match '^Location:' } | Select-Object -Last 1
            if (-not $location) { throw 'Redirect response did not include Location.' }
            $current = ([Uri]::new([Uri]$current, $location.Substring(9).Trim())).AbsoluteUri
            if (-not (Test-RedirectUri $current -AllowedHosts $AllowedHosts -AllowHttp:$AllowHttp)) { throw "Untrusted redirect URL: $current" }
            Remove-Item $headers,$body -Force -ErrorAction SilentlyContinue
        }
    } catch {
        Remove-Item -LiteralPath $OutFile,"$OutFile.headers","$OutFile.part" -Force -ErrorAction SilentlyContinue
        throw
    } finally {
        Remove-Item -LiteralPath $stderrFile -Force -ErrorAction SilentlyContinue
        $ErrorActionPreference = $priorErrorActionPreference
    }
}

function Get-ArchiveSha256 {
    param([string]$Text, [string]$ArchiveName)
    $Text = $Text.TrimStart([char]0xFEFF)
    $found = @()
    foreach ($line in ($Text -split "`r?`n")) {
        if ($line -match '^[ \t\v\f\r]*([0-9a-fA-F]{64})[ \t\v\f\r]+\*?([^ \t\v\f\r]+)[ \t\v\f\r]*$' -and [string]::Equals($Matches[2], $ArchiveName, [StringComparison]::Ordinal)) { $found += $Matches[1].ToLowerInvariant() }
    }
    if (@($found | Select-Object -Unique).Count -gt 1) { throw "conflicting checksums for $ArchiveName" }
    if ($found.Count) { return $found[0] }
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

function Copy-LimitedStream {
    param([IO.Stream]$InputStream, [string]$Destination, [long]$MaxBytes = 104857600)
    $created = $false
    try {
        $output = [IO.File]::Open($Destination, [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
        $created = $true
        try {
            $buffer = New-Object byte[] 81920; $total = 0L
            while (($read = $InputStream.Read($buffer,0,$buffer.Length)) -gt 0) {
                $total += $read
                if ($total -gt $MaxBytes) { throw "Extracted executable exceeds $MaxBytes bytes." }
                $output.Write($buffer,0,$read)
            }
        } finally { $output.Dispose() }
    } catch { if ($created) { Remove-Item -LiteralPath $Destination -Force -ErrorAction SilentlyContinue }; throw }
}

function Get-ZipRootBinary {
    param([string]$ArchivePath, [string]$Destination)
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($ArchivePath)
    try {
        $entries = @($zip.Entries)
        $binaries = @($entries | Where-Object { [string]::Equals($_.FullName, 'gasa.exe', [StringComparison]::Ordinal) })
        if ($binaries.Count -ne 1) { throw 'The release archive must contain exactly one case-sensitive gasa.exe at its root.' }
        foreach ($candidate in $entries) {
            $name = $candidate.FullName
            $unixType = ($candidate.ExternalAttributes -shr 16) -band 0xF000
            $regular = ($unixType -eq 0 -or $unixType -eq 0x8000)
            if (-not $regular -or -not ([string]::Equals($name,'gasa.exe',[StringComparison]::Ordinal) -or [string]::Equals($name,'LICENSE',[StringComparison]::Ordinal) -or [string]::Equals($name,'README.md',[StringComparison]::Ordinal))) { throw "Unsafe or unexpected entry in release archive: $name" }
        }
        $entries = $binaries
        if ($entries.Count -ne 1) { throw 'The release archive must contain exactly one gasa.exe at its root.' }
        $entry = $entries[0]
        $unixType = ($entry.ExternalAttributes -shr 16) -band 0xF000
        if ($entry.FullName.Contains('..') -or $entry.FullName.Contains('/') -or $entry.FullName.Contains('\\') -or [IO.Path]::IsPathRooted($entry.FullName) -or ($unixType -ne 0 -and $unixType -ne 0x8000)) { throw 'Unsafe or non-regular path in release archive.' }
        if ($entry.Length -gt 104857600) { throw 'Extracted executable exceeds 100 MiB.' }
        $input = $entry.Open()
        try { Copy-LimitedStream $input $Destination 104857600 } finally { $input.Dispose() }
    } finally { $zip.Dispose() }
    if (-not (Test-OrdinaryFile $Destination) -or (Get-Item -LiteralPath $Destination).Length -lt 1024) { throw 'Downloaded executable is missing, not a regular file, or too small.' }
}

function Test-BinaryVersion {
    param([string]$Path)
    & $Path --version | Out-Host
    if ($LASTEXITCODE -ne 0) { throw "gasa --version failed for $Path" }
}

function Recover-InstallState {
    param([string]$InstallDirectory)
    New-Item -ItemType Directory -Path $InstallDirectory -Force | Out-Null
    Get-ChildItem -LiteralPath $InstallDirectory -Force -ErrorAction SilentlyContinue | Where-Object { $_.Name -like '.gasa-*.exe' } | Remove-Item -Force -ErrorAction Stop
    $target = Join-Path $InstallDirectory 'gasa.exe'
    if (-not (Test-Path -LiteralPath $target)) {
        $backup = Get-ChildItem -LiteralPath $InstallDirectory -Filter 'gasa.exe.old*' -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($backup) { try { Move-Item -LiteralPath $backup.FullName -Destination $target -Force -ErrorAction Stop } catch { throw "Could not restore backup '$($backup.FullName)': $($_.Exception.Message)" } }
    }
}

function Enter-InstallLock {
    param([string]$InstallDirectory)
    $path = Join-Path $InstallDirectory '.gasa-install.lock'
    for ($attempt = 0; $attempt -lt 2; $attempt++) {
        try {
            $stream = [IO.File]::Open($path, [IO.FileMode]::CreateNew, [IO.FileAccess]::ReadWrite, [IO.FileShare]::Read)
            $owner = "$PID $([DateTimeOffset]::UtcNow.ToUnixTimeSeconds())"
            $bytes = [Text.Encoding]::UTF8.GetBytes("$owner`n")
            $stream.Write($bytes, 0, $bytes.Length); $stream.Flush()
            return [pscustomobject]@{ Path=$path; Owner=$owner; Stream=$stream }
        } catch [IO.IOException] {
            $parts = @(); try { $parts = [IO.File]::ReadAllText($path) -split '\s+' } catch { }
            $ownerPid = 0; $stamp = 0L
            if ($parts.Count -ge 2) { [void][int]::TryParse($parts[0], [ref]$ownerPid); [void][long]::TryParse($parts[1], [ref]$stamp) }
            $age = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds() - $stamp
            $alive = $false; if ($ownerPid -gt 0) { try { $null = Get-Process -Id $ownerPid -ErrorAction Stop; $alive = $true } catch { } }
            if ($attempt -eq 0 -and ($age -gt 1800 -or -not $alive)) { Remove-Item -LiteralPath $path -Force -ErrorAction SilentlyContinue; continue }
            throw 'another installation is in progress'
        }
    }
    throw 'another installation is in progress'
}

function Exit-InstallLock {
    param($Lock)
    if ($Lock) {
        $Lock.Stream.Dispose()
        try {
            $currentOwner = [IO.File]::ReadAllText($Lock.Path).TrimEnd("`r","`n")
            if ([string]::Equals($currentOwner, $Lock.Owner, [StringComparison]::Ordinal)) { Remove-Item -LiteralPath $Lock.Path -Force -ErrorAction Stop }
        } catch { }
    }
}

function New-UniqueBackupPath {
    param([string]$Target)
    return "$Target.old.$([DateTime]::UtcNow.ToString('yyyyMMddHHmmssfff')).$([guid]::NewGuid().ToString('N'))"
}

function Remove-OldBackups {
    param([string]$InstallDirectory)
    $cutoff = [DateTime]::UtcNow.AddMinutes(-10)
    Get-ChildItem -LiteralPath $InstallDirectory -Filter 'gasa.exe.old*' -File -ErrorAction SilentlyContinue | Where-Object { $_.LastWriteTimeUtc -lt $cutoff } | ForEach-Object { $backup = $_; try { Remove-Item -LiteralPath $backup.FullName -Force -ErrorAction Stop } catch { Write-Warning "Could not remove backup $($backup.FullName): $($_.Exception.Message)" } }
}

function Normalize-PathEntry {
    param([string]$Value)
    $value = $Value.Trim().Replace('/', '\')
    try { return [IO.Path]::GetFullPath($value).TrimEnd([char]92) } catch { return $value.TrimEnd([char]92) }
}

function Update-UserPath {
    param([string]$Directory, [bool]$Add, [bool]$ValidateLength = $true, $RegistryStore = $null)
    foreach ($character in $Directory.ToCharArray()) { $code = [int]$character; if ($code -lt 32 -or ($code -ge 0x7f -and $code -le 0x9f) -or $code -in @(0x200e,0x200f,0x2028,0x2029) -or ($code -ge 0x202a -and $code -le 0x202e) -or ($code -ge 0x2066 -and $code -le 0x2069)) { throw 'PATH directory contains a prohibited character.' } }
    if ($Directory -match '[;%*?"<>|]') { throw 'PATH directory contains a prohibited character.' }
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
        if ($Add -and $ValidateLength -and $newPath.Length -gt 32000) { throw 'User PATH is near the 32767 character limit; it was not changed.' }
        if ($Add -and $newPath.Length -gt 2048) { Write-Warning "User PATH is long ($($newPath.Length) characters)." }
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
        $installLock = Enter-InstallLock $installDir
        try {
            Remove-Item -LiteralPath $binary -Force -ErrorAction Stop
            Remove-OldBackups $installDir
            Update-UserPath $installDir $false $false
            Write-Host 'Installation removed; data in %APPDATA%\gasacli was preserved.'
        } finally { Exit-InstallLock $installLock }
        return
    }
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    $installLock = Enter-InstallLock $installDir
    try {
    Recover-InstallState $installDir
    $target = Join-Path $installDir 'gasa.exe'
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
        $target = Join-Path $installDir 'gasa.exe'; $old = New-UniqueBackupPath $target; $staged = Join-Path $installDir ('.gasa-' + [guid]::NewGuid().ToString('N') + '.exe')
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
        if ($moved) { Remove-Item -LiteralPath $old -Force -ErrorAction SilentlyContinue }
        Remove-OldBackups $installDir
    } finally { Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue }
    Write-Host 'Installed. Open a new terminal to refresh PATH.'
    } finally { Exit-InstallLock $installLock }
}

if ($env:GASA_SOURCE_ONLY -ne '1') {
    try { Invoke-GasaInstall } catch { Write-Error $_.Exception.Message; exit 1 }
}
