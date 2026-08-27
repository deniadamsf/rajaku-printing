# Print Agent — struk thermal EPPOS EP8081 / RPP02

Jembatan kecil antara aplikasi web dan printer thermal. Browser tidak bisa
menulis ke port COM, jadi halaman POS mengirim struk ke agen ini lewat HTTP
localhost dan agen yang bicara ke printer.

Tanpa dependensi: PowerShell + System.Drawing + Chrome/Edge yang sudah ada di
mesin. Tidak ada `npm install`.

## Kenapa ada — bukan sekadar preferensi

`window.print()` **tidak bisa dipakai** untuk printer ini. Bukan soal CSS,
tapi cacat di jalur driver, dibuktikan 22 Agustus 2026 dengan membedah byte
yang dikirim driver ke port berkas:

```
56x  GS v 0 raster 72 byte x 24 BARIS     <- blok setinggi 24 baris
56x  ESC J maju 21 TITIK                  <- kertas hanya maju 21 titik
```

Selisih 3 titik per blok, menumpuk 56 kali dalam satu struk. Blok raster
saling tindih, printer kehilangan sinkronisasi, dan sisa data tercetak
sebagai karakter acak (`þ Å ù °`). Angka 21 berasal dari 24 × (180/203) —
driver menghitung jarak maju pada 180 dpi sementara datanya 203 dpi.

Agen ini memakai `ESC *` mode 33 dengan `ESC 3 24`, sehingga jarak maju
**persis** sama dengan tinggi data. Uji diagnostik pada printer ini:
batang uji lewat `ESC *` keluar utuh, lewat `GS v 0` keluar cacat.

## Menjalankan

```powershell
powershell -ExecutionPolicy Bypass -File print-agent.ps1
```

Parameter (semua opsional):

| Parameter | Bawaan | Guna |
|---|---|---|
| `-ListenPort` | `9110` | Port HTTP yang didengarkan (hanya 127.0.0.1) |
| `-ComPort` | `\\.\COM3` | Port printer. Cek nomornya di Devices and Printers |
| `-AllowOrigin` | `http://localhost:3000` | Origin yang boleh memanggil (CORS) |
| `-Threshold` | `175` | Ambang hitam untuk TEKS — naikkan agar lebih tebal |
| `-LogoRows` | `170` | Berapa baris pertama yang dianggap area logo |
| `-LogoThreshold` | `100` | Ambang hitam untuk LOGO — turunkan agar lebih tipis |

Ambangnya sengaja dua nilai. Satu nilai tidak bisa melayani keduanya:
menaikkannya menebalkan huruf tapi menelan garis putih halus di dalam logo
sampai jadi blok hitam; menurunkannya memperjelas logo tapi huruf jadi tipis.

## API

### `GET /health`

```json
{ "success": true, "data": { "status": "ok", "com_port": "\\\\.\\COM3" } }
```

### `POST /print`

```json
{
  "html": "<!doctype html>…struk lengkap…",
  "width_mm": 80,
  "com_port": "\\\\.\\COM4"
}
```

`html` wajib. `width_mm` 58 atau 80 (bawaan 80). `com_port` menimpa bawaan.

Balasan sukses:

```json
{ "success": true, "data": { "width_mm": 80, "height_dots": 1506, "bytes": 109252, "ms": 4787 } }
```

Balasan gagal memakai envelope yang sama dengan backend Go (CLAUDE.md §22):

```json
{ "success": false, "error": { "code": "GAGAL_CETAK", "message": "…" } }
```

## Alur di dalam

1. Setiap `<img>` diunduh dan ditanam sebagai data URI.
   Wajib: dengan `--screenshot`, Chrome memotret sebelum `<img>` selesai
   diunduh, sehingga logo keluar sebagai ikon gambar rusak. Menanamnya
   menghapus balapan waktu itu, bukan sekadar memperpanjang tunggu.
2. Chrome headless memotret pada lebar piksel CSS yang benar
   (80 mm = 302 px CSS), diperbesar `576/302 = 1.9x` agar setara resolusi
   kepala cetak. **Jangan** samakan lebar viewport dengan jumlah titik —
   struk akan mengisi separuh kiri kertas saja.
3. Ruang putih di bawah dipangkas supaya kertas tidak terbuang.
4. Gambar diubah jadi `ESC *` mode 33, dikirim per potong 2 KB ke port COM.

Gambar struk dibuat saat diminta lalu **dibuang** — tidak disimpan ke
database maupun ke disk secara permanen.

## Catatan pemeliharaan

Berkas `.ps1` ini **wajib disimpan sebagai UTF-8 dengan BOM**. PowerShell 5.1
membaca `.ps1` tanpa BOM sebagai ANSI; em-dash `—` (E2 80 94) jadi `â€”`, dan
byte `0x94` adalah tanda kutip-tutup pintar yang menutup string lebih awal
sehingga seluruh skrip gagal di-parse. Setelah mengedit dengan editor yang
menulis UTF-8 polos, kembalikan BOM-nya:

```powershell
$f = 'print-agent.ps1'
$t = [IO.File]::ReadAllText($f, [Text.Encoding]::UTF8)
[IO.File]::WriteAllText($f, $t, (New-Object Text.UTF8Encoding $true))
```

Jangan bungkus panggilan Chrome dengan `$ErrorActionPreference='Stop'` aktif —
Chrome menulis peringatan tak berbahaya ke stderr dan PowerShell mengubah tiap
baris stderr program native jadi ErrorRecord yang mematikan skrip.

## Batasan yang diketahui

- Hanya Windows (System.Drawing + port COM lewat `CreateFile`).
- Harus jalan di setiap komputer kasir yang mencetak; belum ada pemasangan
  otomatis saat boot.
- Belum ada autentikasi. Aman karena hanya mendengarkan di `127.0.0.1`, tapi
  siapa pun yang bisa menjalankan kode di mesin itu bisa mencetak.
- Kabel USB EP8081 hanya mengisi daya; sambungan data selalu Bluetooth.
