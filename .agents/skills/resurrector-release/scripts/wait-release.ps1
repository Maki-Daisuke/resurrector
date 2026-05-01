#requires -Version 7.0
<#
.SYNOPSIS
  Waits for a Resurrector GitHub Release asset to appear and emits its URL + SHA256.

.DESCRIPTION
  Polls the GitHub Releases REST API (via `gh api`) for the given tag until the
  expected windows-amd64 ZIP asset is uploaded and the API exposes its SHA256
  digest. Then prints a JSON object to stdout with the fields the WinGet
  manifest needs:

    {
      "tag":        "v1.1.4",
      "assetName":  "resurrector-v1.1.4-windows-amd64.zip",
      "url":        "https://github.com/.../resurrector-v1.1.4-windows-amd64.zip",
      "sha256":     "1FAD111D83DE...."   # uppercase, no "sha256:" prefix
    }

  Exits non-zero on timeout or unexpected error. Never downloads the ZIP — the
  SHA256 is taken from the asset's `digest` field as published by GitHub. This
  avoids races where a re-uploaded asset would yield a stale local hash.

.PARAMETER Tag
  Release tag to wait for, e.g. "v1.1.4". Required.

.PARAMETER Repo
  GitHub repo in "owner/name" form. Defaults to "Maki-Daisuke/resurrector".

.PARAMETER AssetPattern
  Asset filename to match. Defaults to "resurrector-<Tag>-windows-amd64.zip".

.PARAMETER TimeoutSeconds
  Max time to wait. Defaults to 1800 (30 min).

.PARAMETER IntervalSeconds
  Polling interval. Defaults to 15.

.EXAMPLE
  ./scripts/wait-release.ps1 -Tag v1.1.4

.EXAMPLE
  $info = ./scripts/wait-release.ps1 -Tag v1.1.4 | ConvertFrom-Json
  $info.url
  $info.sha256
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$Tag,

    [string]$Repo = 'Maki-Daisuke/resurrector',

    [string]$AssetPattern,

    [int]$TimeoutSeconds = 1800,

    [int]$IntervalSeconds = 15
)

$ErrorActionPreference = 'Stop'

if (-not $Tag.StartsWith('v')) {
    $Tag = "v$Tag"
}
if (-not $AssetPattern) {
    $AssetPattern = "resurrector-$Tag-windows-amd64.zip"
}

if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    Write-Error "gh CLI is not available on PATH. Install GitHub CLI and authenticate (gh auth login)."
    exit 2
}

$deadline = (Get-Date).AddSeconds($TimeoutSeconds)
$apiPath = "repos/$Repo/releases/tags/$Tag"

Write-Host "Waiting for release $Tag in $Repo (asset: $AssetPattern, timeout: ${TimeoutSeconds}s)" -ForegroundColor Cyan

while ($true) {
    $now = Get-Date
    if ($now -gt $deadline) {
        Write-Error "Timed out after ${TimeoutSeconds}s waiting for $Tag asset $AssetPattern."
        exit 1
    }

    $remaining = [int]($deadline - $now).TotalSeconds
    Write-Host ("[{0:HH:mm:ss}] polling… ({1}s remaining)" -f $now, $remaining) -ForegroundColor DarkGray

    $raw = $null
    try {
        # gh api exits non-zero with 404 until the release is created. Swallow it.
        $raw = gh api $apiPath 2>$null
    }
    catch {
        $raw = $null
    }

    if ($raw) {
        try {
            $release = $raw | ConvertFrom-Json -Depth 20
        }
        catch {
            Write-Warning "Failed to parse release JSON: $_"
            $release = $null
        }

        if ($release -and $release.assets) {
            $asset = $release.assets | Where-Object { $_.name -eq $AssetPattern } | Select-Object -First 1
            if ($asset) {
                $digest = [string]$asset.digest
                if ($digest -and $digest.StartsWith('sha256:')) {
                    $sha256 = $digest.Substring(7).ToUpperInvariant()
                    $result = [pscustomobject]@{
                        tag       = $Tag
                        assetName = $asset.name
                        url       = $asset.browser_download_url
                        sha256    = $sha256
                    }
                    $result | ConvertTo-Json -Compress
                    exit 0
                }
                else {
                    Write-Host "  asset uploaded but digest not yet published; continuing to poll" -ForegroundColor Yellow
                }
            }
        }
    }

    Start-Sleep -Seconds $IntervalSeconds
}
