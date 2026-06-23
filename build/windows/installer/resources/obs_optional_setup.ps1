param(
    [switch] $DetectOnly,
    [string] $DownloadUrl = "https://github.com/obsproject/obs-studio/releases/download/32.1.2/OBS-Studio-32.1.2-Windows-x64-Installer.exe",
    [string] $ExpectedSha256 = "94d180c1fc481ccc307b95513f795d088d63ac4f61ad3253c2ac0d94d0844110"
)

$ErrorActionPreference = "Stop"

$ExpectedSubject = 'CN="OBS Project, LLC", O="OBS Project, LLC", L=Sheridan, S=Wyoming, C=US, SERIALNUMBER=2023-001272252, OID.2.5.4.15=Private Organization, OID.1.3.6.1.4.1.311.60.2.1.2=Wyoming, OID.1.3.6.1.4.1.311.60.2.1.3=US'
$ExpectedIssuer = 'CN=DigiCert G5 CS ECC SHA384 2021 CA1, O="DigiCert, Inc.", C=US'
$ExpectedThumbprint = "178EB178111547B52B3821A278653EEDC3E5B1F7"

function Get-ObsInstall {
    $roots = @(
        "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*",
        "HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*",
        "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*"
    )
    $entry = Get-ItemProperty $roots -ErrorAction SilentlyContinue |
        Where-Object { $_.DisplayName -like "OBS Studio*" } |
        Select-Object -First 1
    if ($entry) {
        return $entry
    }

    $commonPaths = @(
        (Join-Path $env:ProgramFiles "obs-studio\bin\64bit\obs64.exe"),
        (Join-Path ${env:ProgramFiles(x86)} "obs-studio\bin\64bit\obs64.exe")
    )
    foreach ($path in $commonPaths) {
        if ($path -and (Test-Path -LiteralPath $path)) {
            return [pscustomobject]@{ DisplayName = "OBS Studio"; DisplayIcon = $path }
        }
    }
    return $null
}

try {
    $existing = Get-ObsInstall
    if ($DetectOnly) {
        if ($existing) {
            Write-Output "OBS Studio detected."
            exit 0
        }
        Write-Output "OBS Studio was not detected."
        exit 1
    }

    if ($existing) {
        Write-Output "OBS Studio is already installed. Skipping optional OBS installer."
        exit 0
    }

    $downloadDir = Join-Path $env:TEMP "refleks-obs-setup"
    New-Item -ItemType Directory -Force -Path $downloadDir | Out-Null
    $installer = Join-Path $downloadDir "OBS-Studio-32.1.2-Windows-x64-Installer.exe"

    Write-Output "Downloading official OBS Studio installer..."
    Invoke-WebRequest -Uri $DownloadUrl -OutFile $installer

    $actualHash = (Get-FileHash -Algorithm SHA256 -LiteralPath $installer).Hash.ToLowerInvariant()
    if ($actualHash -ne $ExpectedSha256.ToLowerInvariant()) {
        Write-Output "OBS installer verification failed: SHA-256 mismatch."
        exit 0
    }

    $signature = Get-AuthenticodeSignature -LiteralPath $installer
    if ($signature.Status -ne "Valid" -or -not $signature.SignerCertificate) {
        Write-Output "OBS installer verification failed: Authenticode signature is not valid."
        exit 0
    }
    if ($signature.SignerCertificate.Subject -ne $ExpectedSubject) {
        Write-Output "OBS installer verification failed: unexpected signer subject."
        exit 0
    }
    if ($signature.SignerCertificate.Issuer -ne $ExpectedIssuer) {
        Write-Output "OBS installer verification failed: unexpected signer issuer."
        exit 0
    }
    if ($signature.SignerCertificate.Thumbprint -ne $ExpectedThumbprint) {
        Write-Output "OBS installer verification failed: unexpected signer thumbprint."
        exit 0
    }

    Write-Output "OBS installer verified. Launching official OBS Studio setup..."
    $process = Start-Process -FilePath $installer -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        Write-Output "OBS installer exited with code $($process.ExitCode). RefleK's is still installed; finish OBS setup later from obsproject.com."
        exit 0
    }

    Write-Output "OBS Studio setup completed."
    exit 0
} catch {
    if ($DetectOnly) {
        Write-Output "OBS detection failed: $($_.Exception.Message)"
        exit 1
    }
    Write-Output "Optional OBS setup could not complete: $($_.Exception.Message)"
    Write-Output "RefleK's is still installed; finish OBS setup later from obsproject.com."
    exit 0
}
