<#
.SYNOPSIS
    Removes the agentsync --watch sign-in task created by install-watcher.ps1.

.DESCRIPTION
    Deletes the per-user scheduled task (default name agentsync-watch) and
    stops any running agentsync process launched from the executable recorded
    in that task, mirroring "disable --now" semantics. It removes exactly what
    install-watcher.ps1 created: the task registration. No other tasks, files,
    or configuration are touched. A missing task and no matching process both
    report success, so the script is idempotent.

    Task management uses schtasks.exe; process matching uses Get-Process only,
    so the script works on Windows PowerShell 5.1 and PowerShell 7 without CIM.

.PARAMETER AgentsyncPath
    Full path to the agentsync executable. When the task still exists, its own
    recorded executable path takes precedence. This parameter is a fallback for
    resolving which processes to stop. Defaults to the first agentsync found
    on PATH; when agentsync is no longer on PATH, only the task registration
    is removed.

.PARAMETER TaskName
    Scheduled task name. Defaults to agentsync-watch.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File contrib\windows\uninstall-watcher.ps1
#>
[CmdletBinding()]
param(
    [string]$AgentsyncPath = "",
    [string]$TaskName = "agentsync-watch"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
[Console]::OutputEncoding = [Text.Encoding]::UTF8
$ProgressPreference = "SilentlyContinue"

function Invoke-SchtasksCapture {
    param([string[]]$SchtasksArgs)
    # Native stderr becomes terminating under the script's Stop preference on
    # Windows PowerShell 5.1, so relax it in this scope only and judge the
    # call by the exit code. Assignment here is scope-local and vanishes on return.
    $ErrorActionPreference = "Continue"
    $text = & schtasks @SchtasksArgs 2>&1 | Out-String
    return @{ Output = $text; ExitCode = $LASTEXITCODE }
}

function Get-TaskExePath {
    param([string]$Name)
    $result = Invoke-SchtasksCapture -SchtasksArgs @("/query", "/tn", $Name, "/xml")
    if ($result.ExitCode -ne 0) {
        return ""
    }
    try {
        [xml]$doc = $result.Output
    } catch {
        return ""
    }
    $actionArgs = $doc.GetElementsByTagName("Arguments") | Select-Object -First 1
    if ($null -eq $actionArgs) {
        return ""
    }
    $match = [regex]::Match($actionArgs.InnerText, "-FilePath '([^']+)'")
    if ($match.Success) {
        return $match.Groups[1].Value
    }
    return ""
}

function Resolve-AgentsyncPath {
    param([string]$Hint)
    if ($Hint -ne "") {
        if (-not (Test-Path -LiteralPath $Hint -PathType Leaf)) {
            throw "agentsync executable not found: $Hint"
        }
        return (Resolve-Path -LiteralPath $Hint).Path
    }
    $found = Get-Command agentsync -CommandType Application -ErrorAction SilentlyContinue |
        Select-Object -First 1
    if ($null -eq $found) {
        return ""
    }
    return $found.Source
}

$result = Invoke-SchtasksCapture -SchtasksArgs @("/query", "/tn", $TaskName)
if ($result.ExitCode -ne 0) {
    Write-Output "agentsync watcher task '$TaskName' is not registered; nothing to do."
} else {
    $recordedExe = Get-TaskExePath -Name $TaskName
    $result = Invoke-SchtasksCapture -SchtasksArgs @("/delete", "/tn", $TaskName, "/f")
    if ($result.ExitCode -ne 0) {
        throw "failed to remove task '$TaskName': $($result.Output)"
    }
    Write-Output "Unregistered agentsync watcher task '$TaskName'."
    if ($recordedExe -ne "") {
        $AgentsyncPath = $recordedExe
    }
}

$exe = Resolve-AgentsyncPath -Hint $AgentsyncPath
if ($exe -eq "") {
    Write-Output "agentsync not found on PATH; skipped process check."
    exit 0
}
$exeName = [IO.Path]::GetFileNameWithoutExtension($exe)
$stopped = 0
foreach ($proc in @(Get-Process -Name $exeName -ErrorAction SilentlyContinue)) {
    try {
        $path = $proc.Path
    } catch {
        continue
    }
    if ($path -ceq $exe) {
        Stop-Process -Id $proc.Id -Force
        $stopped++
    }
}
if ($stopped -gt 0) {
    Write-Output "Stopped $stopped running agentsync process(es) for $exe."
} else {
    Write-Output "No running agentsync process found for $exe."
}
