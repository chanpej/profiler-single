param([string]$Executable = (Join-Path $PSScriptRoot '../bin/unityperf.exe'))

$ErrorActionPreference = 'Stop'
$executablePath = (Resolve-Path -LiteralPath $Executable).Path
$port = 37100

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
