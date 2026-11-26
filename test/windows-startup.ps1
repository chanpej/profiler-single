param([string]$Executable = (Join-Path $PSScriptRoot '../bin/unityperf.exe'))

$ErrorActionPreference = 'Stop'
$executablePath = (Resolve-Path -LiteralPath $Executable).Path
$port = 37100
Add-Type -AssemblyName UIAutomationClient,UIAutomationTypes
function Wait-Frontend($process) {
    $deadline = [DateTime]::UtcNow.AddSeconds(15)
    $name = [System.Windows.Automation.PropertyCondition]::new([System.Windows.Automation.AutomationElement]::NameProperty, 'App')
    do {
        $window = [System.Windows.Automation.AutomationElement]::FromHandle($process.MainWindowHandle)
        if ($window.FindFirst([System.Windows.Automation.TreeScope]::Descendants, $name)) { return }
        Start-Sleep -Milliseconds 200
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'Window opened but the connection toolbar did not render.'
}

foreach ($portOccupied in @($false, $true)) {
    $portHolder = $null
    $application = $null
    try {
        if ($portOccupied) {
            $portHolder = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Any, $port)
            $portHolder.Start()
        }

        $application = Start-Process -FilePath $executablePath -WorkingDirectory (Split-Path $executablePath) -PassThru
        $timer = [Diagnostics.Stopwatch]::StartNew()
        do {
            Start-Sleep -Milliseconds 200
            $application.Refresh()
        } until ($application.HasExited -or $application.MainWindowTitle -eq 'unityperf' -or $timer.Elapsed.TotalSeconds -ge 15)

        if ($application.HasExited -or $application.MainWindowHandle -eq [IntPtr]::Zero -or $application.MainWindowTitle -ne 'unityperf') {
            throw "unityperf did not display its main window (port occupied: $portOccupied)."
        }

        Wait-Frontend $application
        $listener = Get-NetTCPConnection -LocalPort $port -State Listen -OwningProcess $application.Id -ErrorAction SilentlyContinue
        if ([bool]$listener -eq $portOccupied) {
            throw "Unexpected autoconnect listener state (port occupied: $portOccupied)."
        }
        if (!$application.CloseMainWindow() -or !$application.WaitForExit(5000) -or $application.ExitCode -ne 0) {
            throw "unityperf did not exit cleanly (port occupied: $portOccupied)."
        }
        Write-Output "Windows startup passed (port occupied: $portOccupied, window after $($timer.ElapsedMilliseconds) ms)."
    } finally {
        if ($application -and !$application.HasExited) {
            $application.Kill()
            $application.WaitForExit()
        }
        if ($portHolder) {
            $portHolder.Stop()
        }
    }
}
