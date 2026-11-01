<#
.SYNOPSIS
    Registers agentsync --watch to run at user sign-in with no visible console window.

.DESCRIPTION
    Creates a per-user scheduled task (default name agentsync-watch) that runs
    "agentsync --watch" at the current user's sign-in. No administrator rights
    are needed: the task uses a LogonTrigger scoped to the current user with
    the interactive logon type ("run only when the user is logged on"), so no
    password is stored.

    The task action launches Windows PowerShell with -WindowStyle Hidden, which
    in turn starts agentsync with Start-Process -WindowStyle Hidden. Both layers
    stay hidden, so no console window appears on the desktop. This avoids the
    deprecated VBScript (wscript.exe) launcher approach.

    Task management uses schtasks.exe with an explicit task XML definition, so
    the script works on Windows PowerShell 5.1 and PowerShell 7 without CIM.

    The script is idempotent: if the task already exists with the same
    definition it reports up-to-date and changes nothing; if it exists with a
    different definition it is re-created.

.PARAMETER AgentsyncPath
    Full path to the agentsync executable. Defaults to the first agentsync
    found on PATH.

.PARAMETER TaskName
    Scheduled task name. Defaults to agentsync-watch.

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File contrib\windows\install-watcher.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File contrib\windows\install-watcher.ps1 -AgentsyncPath "$env:LOCALAPPDATA\Programs\agentsync\bin\agentsync.exe"
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
        throw "agentsync not found on PATH; pass -AgentsyncPath explicitly."
    }
    return $found.Source
}

function Get-TaskActionArgument {
    param([string]$ExePath)
    $exeDir = Split-Path -Parent $ExePath
    return "-NoProfile -NonInteractive -WindowStyle Hidden -ExecutionPolicy Bypass -Command `"Start-Process -FilePath '$ExePath' -ArgumentList '--watch' -WindowStyle Hidden -WorkingDirectory '$exeDir'`""
}

function Get-TaskXml {
    param([string]$User, [string]$Arguments, [string]$Description)
    $escUser = [System.Security.SecurityElement]::Escape($User)
    $escArgs = [System.Security.SecurityElement]::Escape($Arguments)
    $escDesc = [System.Security.SecurityElement]::Escape($Description)
    return @"
<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Author>$escUser</Author>
    <Description>$escDesc</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>$escUser</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>$escUser</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <RunOnlyIfIdle>false</RunOnlyIfIdle>
    <WakeToRun>false</WakeToRun>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
    <RestartOnFailure>
      <Count>3</Count>
      <Interval>PT1M</Interval>
    </RestartOnFailure>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>powershell.exe</Command>
      <Arguments>$escArgs</Arguments>
    </Exec>
  </Actions>
</Task>
"@
}

function Invoke-SchtasksCapture {
    param([string[]]$SchtasksArgs)
    # Native stderr becomes terminating under the script's Stop preference on
    # Windows PowerShell 5.1, so relax it in this scope only and judge the
    # call by the exit code. Assignment here is scope-local and vanishes on return.
    $ErrorActionPreference = "Continue"
    $text = & schtasks @SchtasksArgs 2>&1 | Out-String
    return @{ Output = $text; ExitCode = $LASTEXITCODE }
}

function Get-ExistingTaskXml {
    param([string]$Name)
    $result = Invoke-SchtasksCapture -SchtasksArgs @("/query", "/tn", $Name, "/xml")
    if ($result.ExitCode -ne 0) {
        return ""
    }
    return $result.Output
}

function Test-TaskCurrent {
    param([string]$XmlText, [string]$User, [string]$UserSid, [string]$Arguments)
    try {
        [xml]$doc = $XmlText
    } catch {
        return $false
    }
    # schtasks normalizes the definition on import: the principal UserId comes
    # back as a SID and the default LeastPrivilege RunLevel may be omitted.
    $command = $doc.GetElementsByTagName("Command") | Select-Object -First 1
    $actionArgs = $doc.GetElementsByTagName("Arguments") | Select-Object -First 1
    $triggerUser = $doc.GetElementsByTagName("LogonTrigger") | Select-Object -First 1
    $principal = $doc.GetElementsByTagName("Principal") | Select-Object -First 1
    $settings = $doc.GetElementsByTagName("Settings") | Select-Object -First 1
    if ($null -eq $command -or $null -eq $actionArgs -or $null -eq $triggerUser -or
        $null -eq $principal -or $null -eq $settings) {
        return $false
    }
    $runLevelNode = $principal.GetElementsByTagName("RunLevel") | Select-Object -First 1
    $runLevel = "LeastPrivilege"
    if ($null -ne $runLevelNode) {
        $runLevel = $runLevelNode.InnerText
    }
    return ($command.InnerText -eq "powershell.exe") -and
        ($actionArgs.InnerText -eq $Arguments) -and
        (($triggerUser.UserId -eq $User) -or ($triggerUser.UserId -eq $UserSid)) -and
        (($principal.UserId -eq $User) -or ($principal.UserId -eq $UserSid)) -and
        ($principal.LogonType -eq "InteractiveToken") -and
        ($runLevel -eq "LeastPrivilege") -and
        ($settings.MultipleInstancesPolicy -eq "IgnoreNew") -and
        ($settings.ExecutionTimeLimit -eq "PT0S")
}

$exe = Resolve-AgentsyncPath -Hint $AgentsyncPath
$who = [System.Security.Principal.WindowsIdentity]::GetCurrent().Name
$whoSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$arguments = Get-TaskActionArgument -ExePath $exe
$description = "agentsync --watch at user sign-in"

$existing = Get-ExistingTaskXml -Name $TaskName
if (($existing -ne "") -and (Test-TaskCurrent -XmlText $existing -User $who -UserSid $whoSid -Arguments $arguments)) {
    Write-Output "agentsync watcher task '$TaskName' is already registered and up to date."
    exit 0
}
if ($existing -ne "") {
    $result = Invoke-SchtasksCapture -SchtasksArgs @("/delete", "/tn", $TaskName, "/f")
    if ($result.ExitCode -ne 0) {
        throw "failed to remove outdated task '$TaskName': $($result.Output)"
    }
    Write-Output "Removed outdated agentsync watcher task '$TaskName'; re-creating."
}

$xmlPath = Join-Path ([IO.Path]::GetTempPath()) "agentsync-watch-task.xml"
try {
    [IO.File]::WriteAllText($xmlPath, (Get-TaskXml -User $who -Arguments $arguments -Description $description), [Text.Encoding]::Unicode)
    $result = Invoke-SchtasksCapture -SchtasksArgs @("/create", "/tn", $TaskName, "/xml", $xmlPath)
    if ($result.ExitCode -ne 0) {
        throw "failed to register task '$TaskName': $($result.Output)"
    }
} finally {
    Remove-Item -LiteralPath $xmlPath -Force -ErrorAction SilentlyContinue
}
Write-Output "Registered agentsync watcher task '$TaskName' for $who."
Write-Output "It runs '$exe --watch' at sign-in with no console window."
Write-Output "To start it now: schtasks /run /tn '$TaskName'"
