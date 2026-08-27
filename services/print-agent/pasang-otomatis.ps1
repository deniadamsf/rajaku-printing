<#
.SYNOPSIS
  Daftarkan agen cetak supaya otomatis jalan tiap kali komputer kasir ini
  dinyalakan/login, tersembunyi (tanpa jendela), dan otomatis pulih sendiri
  kalau proses berhenti tak terduga.

.DESCRIPTION
  Sebelum ada script ini, agen harus dijalankan manual tiap hari lewat
  `print-agent.ps1` dan jendelanya WAJIB dibiarkan terbuka — kalau tak
  sengaja tertutup, cetak struk diam-diam balik ke jalur driver Windows lama
  yang rusak (lihat README.md § "Kenapa ada").

  Script ini mendaftarkan Windows Scheduled Task bernama "RajakuPrintAgent"
  yang:
    - Jalan otomatis saat user ini login ke Windows (tidak perlu buka apa
      pun secara manual lagi).
    - Berjalan TERSEMBUNYI (tanpa jendela PowerShell yang bisa tak sengaja
      ditutup oleh kasir).
    - Dicek ulang tiap 1 menit — kalau prosesnya mati/crash, Task Scheduler
      menjalankannya lagi otomatis (idem-poten: kalau masih hidup, dilewati,
      tidak dobel).
    - Mencatat semua output ke file log, supaya tetap bisa ditelusuri
      meskipun jendelanya tidak terlihat.

.PARAMETER ComPort
  Port COM printer (lihat CARA-PASANG.txt langkah 2). Bawaan '\\.\COM3'.

.PARAMETER ListenPort
  Port HTTP agen. Bawaan 9110, samakan dengan runtimeConfig frontend
  kalau pernah diubah dari bawaan.

.EXAMPLE
  Klik kanan file ini -> "Run with PowerShell" sebagai Administrator:
    powershell -ExecutionPolicy Bypass -File pasang-otomatis.ps1
    powershell -ExecutionPolicy Bypass -File pasang-otomatis.ps1 -ComPort '\\.\COM4'

.NOTES
  Membatalkan pemasangan otomatis ini (agen tidak akan hilang, cuma
  berhenti auto-start — file print-agent.ps1 tetap bisa dijalankan manual
  seperti biasa):
    Unregister-ScheduledTask -TaskName 'RajakuPrintAgent' -Confirm:$false

  Melihat log kalau ada keluhan cetak setelah dipasang otomatis:
    Get-Content "$env:LOCALAPPDATA\RajakuPrintAgent\agent.log" -Tail 50
#>
[CmdletBinding()]
param(
  [string]$ComPort   = '\\.\COM3',
  [int]$ListenPort   = 9110
)

$ErrorActionPreference = 'Stop'

$currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (-not $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw "Jalankan ulang sebagai Administrator (klik kanan PowerShell -> Run as administrator), lalu ulangi perintahnya. Mendaftarkan tugas otomatis butuh hak admin; agennya sendiri nanti tetap jalan sebagai user biasa."
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$agentPath = Join-Path $scriptDir 'print-agent.ps1'
if (-not (Test-Path $agentPath)) {
  throw "print-agent.ps1 tidak ditemukan di folder yang sama dengan script ini ($scriptDir). Taruh pasang-otomatis.ps1 satu folder dengan print-agent.ps1."
}

$logDir = Join-Path $env:LOCALAPPDATA 'RajakuPrintAgent'
New-Item -ItemType Directory -Path $logDir -Force | Out-Null
$logPath = Join-Path $logDir 'agent.log'

# Dibungkus cmd.exe supaya redirect ">>" ke file log jalan apa adanya (kalau
# dibungkus lewat operator PowerShell, prompt & metadata ikut tercatat,
# bukan cuma Write-Host dari agennya).
$innerCmd = "powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File `"$agentPath`" -ComPort '$ComPort' -ListenPort $ListenPort >> `"$logPath`" 2>&1"
$action = New-ScheduledTaskAction -Execute 'cmd.exe' -Argument "/c `"$innerCmd`""

# Trigger ganda: langsung jalan saat login, DAN dicoba ulang tiap 1 menit
# selamanya. `-MultipleInstances IgnoreNew` membuat percobaan tiap menit ini
# jadi pemeriksaan-kesehatan gratis: kalau agen masih hidup, dilewati; kalau
# sudah mati (crash/di-Stop paksa), langsung jalan lagi dalam <=1 menit —
# tanpa perlu mengandalkan "restart on failure" Task Scheduler yang kadang
# tidak terpicu untuk proses yang mati karena dibunuh, bukan exit code.
$logonTrigger  = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME
$healthTrigger = New-ScheduledTaskTrigger -Once -At (Get-Date) `
  -RepetitionInterval (New-TimeSpan -Minutes 1) -RepetitionDuration ([TimeSpan]::MaxValue)

$settings = New-ScheduledTaskSettingsSet `
  -MultipleInstances IgnoreNew `
  -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
  -ExecutionTimeLimit ([TimeSpan]::Zero) `
  -StartWhenAvailable

# Jalan sebagai user yang sedang login (bukan SYSTEM) dan TIDAK elevated —
# printer thermal tersambung lewat Bluetooth milik akun user ini, dan
# print-agent.ps1 memang didesain jalan sebagai user biasa (lihat README.md).
$principal = New-ScheduledTaskPrincipal -UserId $env:USERNAME -LogonType Interactive -RunLevel Limited

Register-ScheduledTask -TaskName 'RajakuPrintAgent' `
  -Action $action -Trigger @($logonTrigger, $healthTrigger) -Settings $settings -Principal $principal `
  -Description 'Agen cetak struk thermal Rajaku Printing (ESC*) - jalan otomatis saat login, pulih sendiri tiap <=1 menit kalau mati. Lihat services/print-agent/README.md.' `
  -Force | Out-Null

Write-Host ""
Write-Host "  Terdaftar sebagai tugas otomatis: RajakuPrintAgent" -ForegroundColor Green
Write-Host "  - Jalan tiap kali $env:USERNAME login ke komputer ini, tersembunyi."
Write-Host "  - Auto-pulih dalam <=1 menit kalau prosesnya mati."
Write-Host "  - Log: $logPath"
Write-Host ""
Write-Host "  Menjalankan sekarang juga (tidak perlu logout/login dulu)..."
Start-ScheduledTask -TaskName 'RajakuPrintAgent'
Start-Sleep -Seconds 2
try {
  $resp = Invoke-RestMethod -Uri "http://127.0.0.1:$ListenPort/health" -TimeoutSec 5
  if ($resp.success) {
    Write-Host "  OK - agen merespons di http://127.0.0.1:$ListenPort/ (com_port: $($resp.data.com_port))" -ForegroundColor Green
  }
} catch {
  Write-Host "  Belum merespons dalam 2 detik - normal kalau Chrome/printer butuh waktu lebih." -ForegroundColor Yellow
  Write-Host "  Cek: Get-Content `"$logPath`" -Tail 30" -ForegroundColor Yellow
}
Write-Host ""
