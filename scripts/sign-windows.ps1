param([Parameter(Mandatory=$true)][string]$Path)
$ErrorActionPreference = 'Stop'
if (-not $env:TUNNELIX_WINDOWS_CERTIFICATE) { throw 'Windows signing certificate is required' }
$tool = (Get-Command signtool.exe -ErrorAction Stop).Source
& $tool sign /fd SHA256 /tr 'http://timestamp.digicert.com' /td SHA256 /f $env:TUNNELIX_WINDOWS_CERTIFICATE /p $env:TUNNELIX_WINDOWS_CERTIFICATE_PASSWORD $Path
if ($LASTEXITCODE -ne 0) { throw 'Authenticode signing failed' }
& $tool verify /pa $Path
if ($LASTEXITCODE -ne 0) { throw 'Authenticode verification failed' }
