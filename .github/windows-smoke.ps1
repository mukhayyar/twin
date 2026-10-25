# Windows smoke test: makes a real twin of a stand-in app, launches its Start
# Menu shortcut through the shell, and checks the app ran in its own profile.
$ErrorActionPreference = "Stop"
go build -o twin.exe ./cmd/twin
go build -o probe.exe ./.github/windows-probe

.\twin.exe -y "$PWD\probe.exe" Work
if ($LASTEXITCODE -ne 0) { throw "twin failed" }

$lnk = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Twin\probe-Work.lnk"
if (-not (Test-Path $lnk)) { throw "no shortcut at $lnk" }
$ico = Join-Path $env:LOCALAPPDATA "twin\icons\probe-Work.ico"
if (Test-Path $ico) { "icon: $((Get-Item $ico).Length) bytes" } else { "icon: none (kept the original)" }

$env:TWIN_PROBE_OUT = Join-Path $env:RUNNER_TEMP "probe.txt"
Start-Process -FilePath $lnk
for ($i = 0; $i -lt 30 -and -not (Test-Path $env:TWIN_PROBE_OUT); $i++) { Start-Sleep -Seconds 1 }
if (-not (Test-Path $env:TWIN_PROBE_OUT)) { throw "the twin never ran" }

$seen = Get-Content $env:TWIN_PROBE_OUT
$seen
$profileDir = Join-Path $env:USERPROFILE ".app_profiles\probe-Work"
foreach ($want in @("USERPROFILE=$profileDir", "APPDATA=$profileDir\AppData\Roaming",
                    "LOCALAPPDATA=$profileDir\AppData\Local", "TEMP=$profileDir\AppData\Local\Temp")) {
  if (-not ($seen -contains $want)) { throw "expected $want" }
}
"windows twin isolated"
