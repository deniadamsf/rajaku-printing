<#
.SYNOPSIS
  Agen cetak struk thermal untuk EPPOS EP8081 / RPP02.

.DESCRIPTION
  Jembatan kecil antara aplikasi web dan printer thermal. Browser TIDAK BISA
  menulis ke port COM, jadi halaman POS mengirim struk ke agen ini lewat HTTP
  localhost, lalu agen yang bicara ke printer.

  KENAPA TIDAK LEWAT DRIVER WINDOWS SAJA (window.print()):
  Driver POS-80 mencetak gambar dengan perintah `GS v 0` setinggi 24 baris,
  lalu memajukan kertas dengan `ESC J 21` — hanya 21 titik. Selisih 3 titik
  per blok, menumpuk 56 kali dalam satu struk, sehingga blok raster saling
  tindih dan cetakan rusak jadi karakter acak. Angka 21 berasal dari
  24 x (180/203): driver menghitung jarak maju pada 180 dpi sementara datanya
  203 dpi. Dibuktikan 22 Agustus 2026 dengan membedah byte yang dikirim
  driver ke port berkas.

  Agen ini memakai `ESC *` mode 33 dengan `ESC 3 24` (spasi baris = tepat 24
  titik, cocok dengan tinggi data) sehingga tidak pernah melenceng.

  Gambar struk dibuat saat diminta lalu DIBUANG — tidak disimpan ke database
  maupun ke disk secara permanen.

.EXAMPLE
  powershell -ExecutionPolicy Bypass -File print-agent.ps1
  powershell -ExecutionPolicy Bypass -File print-agent.ps1 -ComPort '\\.\COM4' -ListenPort 9110
#>
[CmdletBinding()]
param(
  [int]$ListenPort   = 9110,
  [string]$ComPort   = '\\.\COM3',
  [string[]]$AllowOrigin = @('http://localhost:3000'),
  [string]$ChromePath = '',
  # Ambang hitam-putih. Teks perlu ambang TINGGI supaya tebal & pekat; logo
  # perlu ambang RENDAH supaya garis putih halus di dalamnya tidak tertelan
  # jadi blok hitam. Satu nilai tidak bisa melayani keduanya.
  [int]$Threshold     = 175,
  [int]$LogoRows      = 170,
  [int]$LogoThreshold = 100
)

$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing

if (-not ('PrintNative' -as [type])) {
Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public class PrintNative {
  [DllImport("kernel32.dll", SetLastError=true, CharSet=CharSet.Auto)]
  public static extern IntPtr CreateFile(string name, uint access, uint share, IntPtr sec, uint disp, uint flags, IntPtr tmpl);
  [DllImport("kernel32.dll", SetLastError=true)]
  public static extern bool WriteFile(IntPtr h, byte[] buf, uint toWrite, out uint written, IntPtr ov);
  [DllImport("kernel32.dll", SetLastError=true)]
  public static extern bool FlushFileBuffers(IntPtr h);
  [DllImport("kernel32.dll", SetLastError=true)]
  public static extern bool CloseHandle(IntPtr h);
}
'@
}

function Resolve-Chrome {
  if ($ChromePath -and (Test-Path $ChromePath)) { return $ChromePath }
  $candidates = @(
    "$env:ProgramFiles\Google\Chrome\Application\chrome.exe",
    "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
    "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe",
    "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe",
    "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
  )
  foreach ($c in $candidates) { if (Test-Path $c) { return $c } }
  throw 'Chrome/Edge tidak ditemukan. Jalankan ulang dengan -ChromePath "C:\path\ke\chrome.exe".'
}

# Unduh setiap <img> lalu tanam sebagai data URI.
#
# KENAPA PERLU: dengan --screenshot, Chrome memotret segera setelah halaman
# dianggap siap dan TIDAK menunggu <img> yang masih diunduh — logo struk
# keluar sebagai ikon gambar rusak, sementara CSS & font (yang memblokir
# render) tetap termuat. Menambah waktu tunggu cuma memindahkan taruhan;
# menanam gambarnya menghapus balapan waktu itu sepenuhnya.
function Convert-ImagesToDataUri {
  param([string]$Html)

  $baseHref = ''
  if ($Html -match '(?i)<base[^>]+href="([^"]+)"') { $baseHref = $Matches[1] }

  $evaluator = [System.Text.RegularExpressions.MatchEvaluator] {
    param($m)
    $src = $m.Groups[2].Value
    if ($src -like 'data:*') { return $m.Value }
    $url = $null
    if ($src -match '^https?://') { $url = $src }
    elseif ($baseHref) { try { $url = ([uri]::new([uri]$baseHref, $src)).AbsoluteUri } catch { $url = $null } }
    if (-not $url) { return $m.Value }
    try {
      $resp = Invoke-WebRequest -Uri $url -UseBasicParsing -TimeoutSec 15
      $ct = $resp.Headers['Content-Type']
      if (-not $ct) { $ct = 'image/png' }
      $b64 = [Convert]::ToBase64String($resp.Content)
      Write-Host ("    gambar ditanam: {0} ({1} byte)" -f $url, $resp.Content.Length) -ForegroundColor DarkGray
      return $m.Groups[1].Value + "data:$ct;base64,$b64" + $m.Groups[3].Value
    }
    catch {
      # Gambar gagal diunduh bukan alasan membatalkan struk — teksnya jauh
      # lebih penting daripada logonya. Dicatat, lalu jalan terus.
      Write-Host ("    PERINGATAN: gagal unduh gambar {0} - {1}" -f $url, $_.Exception.Message) -ForegroundColor Yellow
      return $m.Value
    }
  }
  return [regex]::Replace($Html, '(?i)(<img\b[^>]*\ssrc=")([^"]+)(")', $evaluator)
}

# HTML struk -> Bitmap selebar $DotWidth, tinggi mengikuti isi.
function Get-BitmapFromHtml {
  param([string]$Html, [int]$DotWidth, [int]$WidthMm)

  $tmp = Join-Path ([IO.Path]::GetTempPath()) ("struk-" + [guid]::NewGuid().ToString('N').Substring(0, 10))
  $htmlPath = "$tmp.html"; $pngPath = "$tmp.png"; $profile = "$tmp-prof"
  try {
    # Latar putih dipaksa: printer thermal hanya punya hitam & kosong, dan
    # tema gelap pengguna tidak boleh ikut terbawa jadi blok hitam sekertas.
    $shim = '<style>html,body{margin:0;padding:0;background:#fff!important;color:#000}</style>'
    $prepared = Convert-ImagesToDataUri -Html $Html
    ($prepared -replace '(?i)</head>', "$shim</head>") | Out-File -FilePath $htmlPath -Encoding utf8

    $chrome = Resolve-Chrome

    # Lebar viewport WAJIB dalam piksel CSS, bukan titik printer — kalau
    # disamakan begitu saja, struk yang lebarnya `80mm` (= 302 px CSS) cuma
    # mengisi separuh kiri kertas 576 titik.
    #   80mm -> 80/25.4*96 = 302 px CSS,  lalu diperbesar 576/302 = 1.9x
    # supaya tangkapan layarnya langsung setara resolusi kepala cetak dan
    # hurufnya tetap tajam (bukan hasil memperbesar gambar kecil).
    $cssWidth = [int][Math]::Round($WidthMm / 25.4 * 96)
    $scale = [Math]::Round($DotWidth / $cssWidth, 3)

    # Tinggi 2000 px CSS jauh melebihi struk terpanjang; sisa putihnya
    # dipangkas setelah ini. Lebih murah daripada mengukur lewat CDP.
    $args = @(
      '--headless=new', '--disable-gpu', '--no-first-run', '--no-default-browser-check',
      '--hide-scrollbars',
      # WAJIB: tanpa ini Chrome memotret begitu event load selesai, sebelum
      # logo (<img src="/brand/logo-mono-print.png">) selesai diunduh dari
      # server aplikasi — hasilnya ikon gambar rusak di kepala struk.
      # virtual-time-budget menyuruh Chrome menunggu jaringan & timer dulu.
      '--virtual-time-budget=8000',
      "--force-device-scale-factor=$scale",
      '--default-background-color=FFFFFF',
      "--user-data-dir=$profile",
      "--window-size=$cssWidth,2000",
      "--screenshot=$pngPath",
      "file:///$($htmlPath -replace '\\','/')"
    )
    # Chrome menulis peringatan tak berbahaya ke stderr (mis. DEPRECATED_ENDPOINT
    # dari GCM). Dengan $ErrorActionPreference='Stop', PowerShell membungkus tiap
    # baris stderr program native jadi ErrorRecord dan mematikan skrip — padahal
    # cetakannya sendiri baik-baik saja. Jadi EAP diturunkan HANYA di sekitar
    # panggilan ini; keberhasilan dinilai dari ada/tidaknya berkas PNG.
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    try { & $chrome @args 2>$null | Out-Null } finally { $ErrorActionPreference = $prevEap }
    if (-not (Test-Path $pngPath)) { throw 'Chrome gagal menghasilkan tangkapan layar struk.' }

    $raw = [System.Drawing.Image]::FromFile($pngPath)
    $copy = New-Object System.Drawing.Bitmap($raw)
    $raw.Dispose()
    return (Remove-TrailingWhite -Bitmap $copy)
  }
  finally {
    foreach ($p in @($htmlPath, $pngPath)) { if (Test-Path $p) { [IO.File]::Delete($p) } }
    if (Test-Path $profile) { [IO.Directory]::Delete($profile, $true) }
  }
}

# Buang ruang putih di bawah struk supaya kertas tidak terbuang percuma.
function Remove-TrailingWhite {
  param([System.Drawing.Bitmap]$Bitmap)

  $rect = New-Object System.Drawing.Rectangle(0, 0, $Bitmap.Width, $Bitmap.Height)
  $d = $Bitmap.LockBits($rect, [System.Drawing.Imaging.ImageLockMode]::ReadOnly, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
  $stride = $d.Stride
  $buf = New-Object byte[] ($stride * $Bitmap.Height)
  [System.Runtime.InteropServices.Marshal]::Copy($d.Scan0, $buf, 0, $buf.Length)
  $Bitmap.UnlockBits($d)

  $last = 0
  for ($y = $Bitmap.Height - 1; $y -ge 0; $y--) {
    $base = $y * $stride; $ink = $false
    for ($x = 0; $x -lt $Bitmap.Width; $x++) {
      if ($buf[$base + $x * 3] -lt 200) { $ink = $true; break }
    }
    if ($ink) { $last = $y; break }
  }
  $h = [Math]::Min($Bitmap.Height, $last + 12)   # sisakan sedikit napas di bawah
  if ($h -le 0) { throw 'Struk terlihat kosong (tidak ada piksel gelap sama sekali).' }

  $out = New-Object System.Drawing.Bitmap($Bitmap.Width, $h, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
  $g = [System.Drawing.Graphics]::FromImage($out)
  $g.Clear([System.Drawing.Color]::White)
  $g.DrawImage($Bitmap, 0, 0)
  $g.Dispose(); $Bitmap.Dispose()
  return $out
}

# Bitmap -> byte ESC/POS memakai ESC * m=33 (24 titik, kerapatan ganda).
function Convert-ToEscStar {
  param([System.Drawing.Bitmap]$Bitmap, [int]$DotWidth)

  $src = $Bitmap
  if ($src.Width -ne $DotWidth) {
    $scaled = New-Object System.Drawing.Bitmap($DotWidth, [int][Math]::Round($src.Height * $DotWidth / $src.Width), [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
    $g = [System.Drawing.Graphics]::FromImage($scaled)
    $g.Clear([System.Drawing.Color]::White)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.DrawImage($src, 0, 0, $scaled.Width, $scaled.Height)
    $g.Dispose(); $src.Dispose(); $src = $scaled
  }

  $h = $src.Height
  $rect = New-Object System.Drawing.Rectangle(0, 0, $DotWidth, $h)
  $d = $src.LockBits($rect, [System.Drawing.Imaging.ImageLockMode]::ReadOnly, [System.Drawing.Imaging.PixelFormat]::Format24bppRgb)
  $stride = $d.Stride
  $buf = New-Object byte[] ($stride * $h)
  [System.Runtime.InteropServices.Marshal]::Copy($d.Scan0, $buf, 0, $buf.Length)
  $src.UnlockBits($d); $src.Dispose()

  $bits = New-Object 'System.Collections.BitArray' ($DotWidth * $h)
  for ($y = 0; $y -lt $h; $y++) {
    $base = $y * $stride; $rowOff = $y * $DotWidth
    $thr = if ($y -lt $LogoRows) { $LogoThreshold } else { $Threshold }
    for ($x = 0; $x -lt $DotWidth; $x++) {
      $i = $base + $x * 3
      $lum = ($buf[$i] * 0.114) + ($buf[$i + 1] * 0.587) + ($buf[$i + 2] * 0.299)
      if ($lum -lt $thr) { $bits[$rowOff + $x] = $true }
    }
  }

  $out = New-Object 'System.Collections.Generic.List[byte]'
  $out.AddRange([byte[]]@(0x1B, 0x40))                 # ESC @  init
  $out.AddRange([byte[]]@(0x1B, 0x33, 24))             # ESC 3 24 -> spasi baris = tinggi data
  $nL = [byte]($DotWidth -band 0xFF); $nH = [byte]($DotWidth -shr 8)

  for ($y0 = 0; $y0 -lt $h; $y0 += 24) {
    $out.AddRange([byte[]]@(0x1B, 0x2A, 33, $nL, $nH))
    for ($x = 0; $x -lt $DotWidth; $x++) {
      $b0 = 0; $b1 = 0; $b2 = 0
      for ($k = 0; $k -lt 8; $k++) {
        if ((($y0 + $k) -lt $h) -and $bits[($y0 + $k) * $DotWidth + $x]) { $b0 = $b0 -bor (0x80 -shr $k) }
        if ((($y0 + 8 + $k) -lt $h) -and $bits[($y0 + 8 + $k) * $DotWidth + $x]) { $b1 = $b1 -bor (0x80 -shr $k) }
        if ((($y0 + 16 + $k) -lt $h) -and $bits[($y0 + 16 + $k) * $DotWidth + $x]) { $b2 = $b2 -bor (0x80 -shr $k) }
      }
      $out.Add([byte]$b0); $out.Add([byte]$b1); $out.Add([byte]$b2)
    }
    $out.Add(0x0A)                                      # LF -> maju tepat 24 titik
  }

  $out.AddRange([byte[]]@(0x1B, 0x32))                  # ESC 2 -> spasi baris normal lagi
  $out.AddRange([byte[]]@(0x1B, 0x64, 0x04))            # maju 4 baris agar struk bisa disobek
  return $out.ToArray()
}

function Send-ToPrinter {
  param([byte[]]$Bytes, [string]$Port)

  $h = [PrintNative]::CreateFile($Port, 0x40000000, 0, [IntPtr]::Zero, 3, 0, [IntPtr]::Zero)
  if ($h -eq [IntPtr](-1)) {
    $err = [Runtime.InteropServices.Marshal]::GetLastWin32Error()
    $hint = if ($err -eq 2) { ' (printer mati / Bluetooth putus / nomor COM salah)' } else { '' }
    throw "Tidak bisa membuka $Port, error Windows $err$hint"
  }
  try {
    # Dikirim per potong, bukan sekaligus, supaya buffer printer portable
    # tidak dijejali dalam satu tembakan.
    $chunk = 2048
    for ($i = 0; $i -lt $Bytes.Length; $i += $chunk) {
      $n = [Math]::Min($chunk, $Bytes.Length - $i)
      $part = New-Object byte[] $n
      [Array]::Copy($Bytes, $i, $part, 0, $n)
      $written = 0
      if (-not [PrintNative]::WriteFile($h, $part, [uint32]$n, [ref]$written, [IntPtr]::Zero)) {
        throw ('Gagal menulis ke printer, error Windows {0}' -f [Runtime.InteropServices.Marshal]::GetLastWin32Error())
      }
      [void][PrintNative]::FlushFileBuffers($h)
      Start-Sleep -Milliseconds 25
    }
  }
  finally { [void][PrintNative]::CloseHandle($h) }
}

# ---------------------------------------------------------------- HTTP server
$listener = New-Object System.Net.HttpListener
$listener.Prefixes.Add("http://127.0.0.1:$ListenPort/")
try { $listener.Start() }
catch { throw "Gagal mendengarkan di port $ListenPort. Mungkin agen lain sudah jalan. ($($_.Exception.Message))" }

Write-Host ""
Write-Host "  Agen cetak Rajaku Printing" -ForegroundColor Green
Write-Host "  mendengarkan : http://127.0.0.1:$ListenPort/"
Write-Host "  printer      : $ComPort"
Write-Host "  ambang       : teks=$Threshold  logo=$LogoThreshold (baris 0-$LogoRows)"
Write-Host "  Tekan Ctrl+C untuk berhenti."
Write-Host ""

function Write-Json {
  param($Context, [int]$Status, $Body)
  $res = $Context.Response
  $origin = $Context.Request.Headers['Origin']
  if ($origin -and ($AllowOrigin -contains $origin)) {
    $res.Headers.Add('Access-Control-Allow-Origin', $origin)
  }
  $res.Headers.Add('Access-Control-Allow-Headers', 'Content-Type')
  $res.Headers.Add('Access-Control-Allow-Methods', 'POST, GET, OPTIONS')
  $res.StatusCode = $Status
  $res.ContentType = 'application/json; charset=utf-8'
  $bytes = [Text.Encoding]::UTF8.GetBytes(($Body | ConvertTo-Json -Compress -Depth 5))
  $res.ContentLength64 = $bytes.Length
  $res.OutputStream.Write($bytes, 0, $bytes.Length)
  $res.OutputStream.Close()
}

while ($listener.IsListening) {
  try {
    $ctx = $listener.GetContext()
    $path = $ctx.Request.Url.AbsolutePath
    $method = $ctx.Request.HttpMethod

    if ($method -eq 'OPTIONS') { Write-Json $ctx 204 @{}; continue }

    if ($path -eq '/health' -and $method -eq 'GET') {
      Write-Json $ctx 200 @{ success = $true; data = @{ status = 'ok'; com_port = $ComPort } }
      continue
    }

    if ($path -eq '/print' -and $method -eq 'POST') {
      $reader = New-Object IO.StreamReader($ctx.Request.InputStream, [Text.Encoding]::UTF8)
      $raw = $reader.ReadToEnd(); $reader.Close()
      $req = $raw | ConvertFrom-Json

      if (-not $req.html) { Write-Json $ctx 400 @{ success = $false; error = @{ code = 'HTML_KOSONG'; message = 'Field "html" wajib diisi.' } }; continue }
      $widthMm = if ($req.width_mm) { [int]$req.width_mm } else { 80 }
      $dots = if ($widthMm -eq 58) { 384 } else { 576 }
      $port = if ($req.com_port) { [string]$req.com_port } else { $ComPort }

      $t0 = Get-Date
      $bmp = Get-BitmapFromHtml -Html ([string]$req.html) -DotWidth $dots -WidthMm $widthMm
      $tinggi = $bmp.Height

      # debug=true: simpan raster persis yang akan dicetak, dan (opsional)
      # LEWATI pengiriman ke printer. Dipakai untuk memeriksa hasil tanpa
      # membuang kertas, atau saat printer sedang tidak terhubung.
      $debugPng = $null
      if ($req.debug) {
        $debugPng = Join-Path ([IO.Path]::GetTempPath()) ("struk-debug-" + (Get-Date -Format 'HHmmss') + ".png")
        $bmp.Save($debugPng, [System.Drawing.Imaging.ImageFormat]::Png)
        Write-Host "    debug PNG: $debugPng" -ForegroundColor DarkGray
      }

      $bytes = Convert-ToEscStar -Bitmap $bmp -DotWidth $dots
      if (-not $req.dry_run) { Send-ToPrinter -Bytes $bytes -Port $port }
      $ms = [int]((Get-Date) - $t0).TotalMilliseconds

      Write-Host ("  [{0:HH:mm:ss}] cetak OK — {1}mm, {2} titik tinggi, {3} byte, {4} ms" -f (Get-Date), $widthMm, $tinggi, $bytes.Length, $ms) -ForegroundColor Green
      Write-Json $ctx 200 @{ success = $true; data = @{ width_mm = $widthMm; height_dots = $tinggi; bytes = $bytes.Length; ms = $ms; dry_run = [bool]$req.dry_run; debug_png = $debugPng } }
      continue
    }

    Write-Json $ctx 404 @{ success = $false; error = @{ code = 'TIDAK_DITEMUKAN'; message = "Rute $method $path tidak dikenal." } }
  }
  catch {
    $msg = $_.Exception.Message
    Write-Host ("  [{0:HH:mm:ss}] GAGAL — {1}" -f (Get-Date), $msg) -ForegroundColor Red
    try { Write-Json $ctx 500 @{ success = $false; error = @{ code = 'GAGAL_CETAK'; message = $msg } } } catch {}
  }
}
