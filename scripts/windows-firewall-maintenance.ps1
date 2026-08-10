[CmdletBinding(SupportsShouldProcess)]
param(
    [ValidateSet('Audit', 'Cleanup')]
    [string]$Mode = 'Audit',
    [switch]$Elevate
)

$ErrorActionPreference = 'Stop'

function Test-IsAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Resolve-ProgramPath {
    param([string]$Program)

    if ([string]::IsNullOrWhiteSpace($Program) -or $Program -eq 'Any') {
        return $null
    }
    $expanded = [Environment]::ExpandEnvironmentVariables($Program.Trim('"'))
    try {
        return [IO.Path]::GetFullPath($expanded)
    }
    catch {
        return $expanded
    }
}

function Test-IsWithinRoot {
    param(
        [string]$Path,
        [string]$Root
    )

    if ([string]::IsNullOrWhiteSpace($Path) -or [string]::IsNullOrWhiteSpace($Root)) {
        return $false
    }
    $normalizedRoot = [IO.Path]::GetFullPath($Root).TrimEnd('\') + '\'
    return $Path.StartsWith($normalizedRoot, [StringComparison]::OrdinalIgnoreCase)
}

function Get-MetAPIFirewallCandidates {
    $tempRoots = @(
        $env:TEMP,
        (Join-Path $env:USERPROFILE 'tmp')
    ) | Where-Object { -not [string]::IsNullOrWhiteSpace($_) }
    $legacyRoots = @()

    foreach ($app in Get-NetFirewallApplicationFilter -PolicyStore ActiveStore) {
        $programPath = Resolve-ProgramPath $app.Program
        if (-not $programPath) {
            continue
        }
        $leaf = [IO.Path]::GetFileName($programPath)
        if ($leaf -notmatch '^(?:codex-)?metapi(?:[-_].*)?(?:\.exe)?$') {
            continue
        }

        $reason = $null
        if (-not (Test-Path -LiteralPath $programPath -PathType Leaf)) {
            $reason = 'missing executable'
        }
        elseif ($tempRoots | Where-Object { Test-IsWithinRoot -Path $programPath -Root $_ }) {
            $reason = 'temporary build'
        }
        elseif ($legacyRoots | Where-Object { Test-IsWithinRoot -Path $programPath -Root $_ }) {
            $reason = 'legacy checkout'
        }
        if (-not $reason) {
            continue
        }

        foreach ($rule in @($app | Get-NetFirewallRule) | Where-Object Direction -eq 'Inbound') {
            $port = $rule | Get-NetFirewallPortFilter
            [pscustomobject]@{
                RuleName    = $rule.Name
                DisplayName = $rule.DisplayName
                Enabled     = $rule.Enabled
                Profile     = $rule.Profile
                Protocol    = $port.Protocol
                LocalPort   = $port.LocalPort
                Program     = $programPath
                Reason      = $reason
            }
        }
    }
}

if ($Mode -eq 'Cleanup' -and -not (Test-IsAdministrator)) {
    if (-not $Elevate) {
        throw 'Cleanup requires Administrator. Re-run with -Mode Cleanup -Elevate for one UAC prompt.'
    }
    $hostExe = (Get-Process -Id $PID).Path
    $child = Start-Process -FilePath $hostExe -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList @(
        '-NoProfile',
        '-ExecutionPolicy', 'Bypass',
        '-File', $PSCommandPath,
        '-Mode', 'Cleanup'
    )
    exit $child.ExitCode
}

$candidates = @(Get-MetAPIFirewallCandidates | Sort-Object Program, RuleName -Unique)
if ($candidates.Count -eq 0) {
    Write-Output 'No stale MetAPI inbound firewall rules found.'
}
elseif ($Mode -eq 'Audit') {
    $candidates | Format-Table DisplayName, Enabled, Profile, Protocol, LocalPort, Reason, Program -AutoSize
    Write-Output "Audit only: $($candidates.Count) stale rule(s). Use -Mode Cleanup -Elevate to remove exactly these rules."
}
else {
    $candidates | Format-Table DisplayName, Enabled, Profile, Protocol, LocalPort, Reason, Program -AutoSize
    $removed = 0
    foreach ($candidate in $candidates) {
        if ($PSCmdlet.ShouldProcess($candidate.RuleName, "remove stale MetAPI firewall rule for $($candidate.Program)")) {
            Get-NetFirewallRule -PolicyStore PersistentStore -Name $candidate.RuleName -ErrorAction Stop |
                Remove-NetFirewallRule -ErrorAction Stop
            $removed++
        }
    }
    Write-Output "Removed $removed stale MetAPI inbound firewall rule(s)."
}
