import os
import base64
import subprocess
from io import BytesIO
from PIL import Image

def get_optimized_logo_base64():
    logo_path = 'd:/RAJAKU PRINTING/LOGO.png'
    if not os.path.exists(logo_path):
        return ""
    img = Image.open(logo_path)
    # Resize keeping aspect ratio, width max 800px
    max_w = 800
    if img.width > max_w:
        ratio = max_w / float(img.width)
        new_h = int(float(img.height) * ratio)
        img = img.resize((max_w, new_h), Image.Resampling.LANCZOS)
    
    buffer = BytesIO()
    img.save(buffer, format="PNG", optimize=True)
    return "data:image/png;base64," + base64.b64encode(buffer.getvalue()).decode('utf-8')

logo_b64 = get_optimized_logo_base64()

html_content = f"""<!DOCTYPE html>
<html lang="id">
<head>
<meta charset="UTF-8">
<title>Buku Panduan Penggunaan Web App Rajaku Printing</title>
<style>
  @import url('https://fonts.googleapis.com/css2?family=Fraunces:ital,opsz,wght@0,9..144,400..700;1,9..144,400..700&family=Inter:wght@300;400;500;600;700;800&family=JetBrains+Mono:wght@400;500;600&display=swap');

  @page {{
    size: A4;
    margin: 18mm 16mm 18mm 16mm;
    @bottom-right {{
      content: counter(page);
    }}
  }}

  @page :first {{
    margin: 0;
    @bottom-right {{
      content: normal;
    }}
  }}

  * {{
    box-sizing: border-box;
    -webkit-print-color-adjust: exact !important;
    print-color-adjust: exact !important;
  }}

  body {{
    font-family: 'Inter', system-ui, -apple-system, sans-serif;
    color: #171717;
    background-color: #FAFAF9;
    font-size: 9.5pt;
    line-height: 1.55;
    margin: 0;
    padding: 0;
  }}

  /* Typography */
  h1, h2, h3, h4, .font-serif {{
    font-family: 'Fraunces', Georgia, serif;
    color: #0A0A0A;
    font-weight: 600;
    letter-spacing: -0.015em;
  }}

  .font-mono {{
    font-family: 'JetBrains Mono', Consolas, monospace;
  }}

  /* Page Break Utilities */
  .page-break {{
    page-break-before: always;
    break-before: page;
  }}

  .avoid-break {{
    page-break-inside: avoid;
    break-inside: avoid;
  }}

  /* COVER PAGE */
  .cover-page {{
    width: 100vw;
    height: 100vh;
    min-height: 297mm;
    background: linear-gradient(145deg, #0d0d0d 0%, #1a1111 45%, #2a0e0e 100%);
    color: #FAFAF9;
    padding: 30mm 24mm 24mm 24mm;
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    position: relative;
    overflow: hidden;
  }}

  .cover-accent-circle {{
    position: absolute;
    top: -100px;
    right: -100px;
    width: 450px;
    height: 450px;
    border-radius: 50%;
    background: radial-gradient(circle, rgba(176,141,87,0.18) 0%, rgba(139,26,26,0.08) 50%, transparent 70%);
    pointer-events: none;
  }}

  .cover-logo-wrapper {{
    display: flex;
    align-items: center;
    gap: 16px;
  }}

  .cover-logo-img {{
    height: 68px;
    width: auto;
    object-fit: contain;
    filter: drop-shadow(0 4px 12px rgba(0,0,0,0.5));
  }}

  .cover-badge {{
    display: inline-block;
    padding: 6px 14px;
    background: rgba(176, 141, 87, 0.15);
    border: 1px solid rgba(176, 141, 87, 0.4);
    border-radius: 9999px;
    color: #C9A44A;
    font-size: 8.5pt;
    font-weight: 600;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    margin-bottom: 20px;
  }}

  .cover-title {{
    font-size: 32pt;
    line-height: 1.12;
    color: #FFFFFF;
    margin: 0 0 16px 0;
    font-weight: 700;
  }}

  .cover-title span {{
    color: #C9A44A;
  }}

  .cover-subtitle {{
    font-size: 13pt;
    line-height: 1.5;
    color: #D6D3D1;
    font-weight: 300;
    max-width: 600px;
    margin: 0 0 30px 0;
  }}

  .cover-divider {{
    height: 3px;
    width: 80px;
    background: linear-gradient(90deg, #B08D57 0%, #8B1A1A 100%);
    border-radius: 2px;
    margin-bottom: 30px;
  }}

  .cover-feature-pills {{
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 40px;
  }}

  .cover-pill {{
    background: rgba(255, 255, 255, 0.04);
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 8px;
    padding: 12px;
  }}

  .cover-pill-title {{
    font-size: 9pt;
    font-weight: 600;
    color: #C9A44A;
    margin-bottom: 3px;
  }}

  .cover-pill-desc {{
    font-size: 7.5pt;
    color: #A8A29E;
    line-height: 1.35;
  }}

  .cover-footer {{
    border-top: 1px solid rgba(255, 255, 255, 0.12);
    padding-top: 18px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 8.5pt;
    color: #A8A29E;
  }}

  /* Document Body Layout */
  .doc-container {{
    padding: 0;
  }}

  .header-running {{
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-bottom: 1px solid #E7E5E4;
    padding-bottom: 8px;
    margin-bottom: 22px;
    font-size: 8pt;
    color: #78716C;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }}

  .header-running strong {{
    color: #8B1A1A;
  }}

  .section-header {{
    border-bottom: 2px solid #8B1A1A;
    padding-bottom: 8px;
    margin-top: 28px;
    margin-bottom: 16px;
    display: flex;
    align-items: center;
    justify-content: space-between;
  }}

  .section-title {{
    font-size: 17pt;
    color: #8B1A1A;
    margin: 0;
  }}

  .section-tag {{
    font-size: 8pt;
    font-weight: 600;
    color: #B08D57;
    background: #FAF3F3;
    padding: 4px 10px;
    border-radius: 9999px;
    border: 1px solid rgba(176, 141, 87, 0.3);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }}

  h2 {{
    font-size: 13pt;
    color: #171717;
    margin: 18px 0 10px 0;
  }}

  h3 {{
    font-size: 10.5pt;
    color: #8B1A1A;
    margin: 14px 0 6px 0;
  }}

  p {{
    margin: 0 0 9px 0;
    color: #292524;
  }}

  /* CARDS & PANELS */
  .card {{
    background: #FFFFFF;
    border: 1px solid #E7E5E4;
    border-radius: 8px;
    padding: 14px 16px;
    margin-bottom: 14px;
    box-shadow: 0 1px 3px rgba(0,0,0,0.02);
  }}

  .card-header {{
    font-weight: 700;
    color: #0A0A0A;
    margin-bottom: 6px;
    font-size: 10pt;
    display: flex;
    align-items: center;
    gap: 6px;
  }}

  .grid-2 {{
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px;
    margin-bottom: 14px;
  }}

  .grid-3 {{
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 14px;
  }}

  .grid-4 {{
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 10px;
    margin-bottom: 14px;
  }}

  /* STEP WORKFLOW */
  .step-flow {{
    display: flex;
    align-items: stretch;
    gap: 8px;
    margin: 12px 0 16px 0;
  }}

  .step-node {{
    flex: 1;
    background: #FFFFFF;
    border: 1px solid #E7E5E4;
    border-top: 3px solid #8B1A1A;
    border-radius: 6px;
    padding: 10px 8px;
    text-align: center;
  }}

  .step-node.gold {{
    border-top-color: #B08D57;
  }}

  .step-node.dark {{
    border-top-color: #171717;
  }}

  .step-num {{
    display: inline-block;
    width: 20px;
    height: 20px;
    line-height: 20px;
    border-radius: 50%;
    background: #8B1A1A;
    color: #FFFFFF;
    font-size: 8pt;
    font-weight: bold;
    margin-bottom: 5px;
  }}

  .step-node.gold .step-num {{
    background: #B08D57;
  }}

  .step-node.dark .step-num {{
    background: #171717;
  }}

  .step-text {{
    font-size: 8pt;
    font-weight: 600;
    color: #171717;
    line-height: 1.25;
  }}

  .step-desc {{
    font-size: 7pt;
    color: #78716C;
    margin-top: 3px;
    line-height: 1.2;
  }}

  /* TABLES */
  table {{
    width: 100%;
    border-collapse: collapse;
    margin: 12px 0 16px 0;
    font-size: 8.5pt;
    background: #FFFFFF;
    border-radius: 6px;
    overflow: hidden;
    border: 1px solid #E7E5E4;
  }}

  th {{
    background: #F5F5F4;
    color: #171717;
    text-align: left;
    padding: 8px 12px;
    font-weight: 600;
    border-bottom: 2px solid #E7E5E4;
    font-size: 8pt;
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }}

  td {{
    padding: 8px 12px;
    border-bottom: 1px solid #F0EFEB;
    color: #292524;
    vertical-align: top;
  }}

  tr:last-child td {{
    border-bottom: none;
  }}

  tr:nth-child(even) td {{
    background-color: #FAFAF9;
  }}

  /* BADGES */
  .badge {{
    display: inline-block;
    padding: 2px 7px;
    border-radius: 4px;
    font-size: 7.5pt;
    font-weight: 600;
    text-transform: uppercase;
  }}

  .badge-crimson {{
    background: #FAF3F3;
    color: #8B1A1A;
    border: 1px solid #F5E5E5;
  }}

  .badge-gold {{
    background: #FAF7F0;
    color: #8F7042;
    border: 1px solid #EFE4CC;
  }}

  .badge-green {{
    background: #F0FDF4;
    color: #15803D;
    border: 1px solid #DCFCE7;
  }}

  .badge-blue {{
    background: #EFF6FF;
    color: #1D4ED8;
    border: 1px solid #DBEAFE;
  }}

  .badge-gray {{
    background: #F5F5F4;
    color: #57534E;
    border: 1px solid #E7E5E4;
  }}

  /* CALLOUT BOXES */
  .callout {{
    padding: 11px 14px;
    border-radius: 6px;
    margin: 12px 0;
    border-left: 4px solid #8B1A1A;
    background: #FAF3F3;
    font-size: 8.5pt;
  }}

  .callout.tip {{
    border-left-color: #B08D57;
    background: #FBF8F2;
  }}

  .callout.info {{
    border-left-color: #2563EB;
    background: #F0F6FE;
  }}

  .callout-title {{
    font-weight: 700;
    color: #8B1A1A;
    margin-bottom: 3px;
    display: flex;
    align-items: center;
    gap: 6px;
  }}

  .callout.tip .callout-title {{
    color: #8F7042;
  }}

  .callout.info .callout-title {{
    color: #1D4ED8;
  }}

  /* LISTS */
  ol, ul {{
    margin: 0 0 10px 0;
    padding-left: 18px;
  }}

  li {{
    margin-bottom: 4px;
  }}

  /* ICONS (inline SVG) */
  .icon {{
    display: inline-block;
    width: 14px;
    height: 14px;
    vertical-align: middle;
    stroke-width: 2;
    stroke: currentColor;
    fill: none;
    stroke-linecap: round;
    stroke-linejoin: round;
  }}

  /* ROLE CARD */
  .role-card {{
    background: #FFFFFF;
    border: 1px solid #E7E5E4;
    border-radius: 8px;
    padding: 12px 14px;
    margin-bottom: 10px;
  }}
  .role-header {{
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 6px;
  }}
  .role-title {{
    font-weight: 700;
    color: #8B1A1A;
    font-size: 10pt;
  }}
  .role-menu {{
    font-size: 7.5pt;
    color: #78716C;
    background: #F5F5F4;
    padding: 2px 8px;
    border-radius: 4px;
    font-family: 'JetBrains Mono', monospace;
  }}

</style>
</head>
<body>

<!-- ========================================================== -->
<!-- 1. HALAMAN COVER -->
<!-- ========================================================== -->
<div class="cover-page">
  <div class="cover-accent-circle"></div>
  
  <div>
    <div class="cover-logo-wrapper">
      <img src="{logo_b64}" alt="Rajaku Printing Logo" class="cover-logo-img">
    </div>
  </div>

  <div style="position: relative; z-index: 10;">
    <div class="cover-badge">Buku Panduan Operasional & Penggunaan Resmi</div>
    <h1 class="cover-title">Panduan Lengkap<br><span>Web App Rajaku Printing</span></h1>
    <div class="cover-divider"></div>
    <p class="cover-subtitle">
      Panduan praktis, terstruktur, dan mudah dipahami untuk Pelanggan, Kasir Toko, Staf Keuangan, Desainer, Operator Mesin Cetak, dan Pemilik Toko.
    </p>

    <div class="cover-feature-pills">
      <div class="cover-pill">
        <div class="cover-pill-title">🛒 Jalur Pelanggan</div>
        <div class="cover-pill-desc">Order online tanpa login, estimasi harga instan, upload desain, & lacak resi real-time.</div>
      </div>
      <div class="cover-pill">
        <div class="cover-pill-title">⚡ Kasir POS Cepat</div>
        <div class="cover-pill-desc">Input walk-in, kenali no. WA otomatis, bayar tunai/QRIS, cetak struk thermal.</div>
      </div>
      <div class="cover-pill">
        <div class="cover-pill-title">🏭 Workflow Pabrik</div>
        <div class="cover-pill-desc">Desain ACC, antrean cetak mesin, kendali mutu QC, & notifikasi WA otomatis.</div>
      </div>
    </div>
  </div>

  <div class="cover-footer">
    <div>Rajaku Printing — Percetakan Banner & Media Promosi Modern</div>
    <div>Edisi 2026 • Versi 1.0</div>
  </div>
</div>

<!-- ========================================================== -->
<!-- 2. DAFTAR ISI & PENGENALAN -->
<!-- ========================================================== -->
<div class="doc-container page-break" style="padding: 10px 0;">
  <div class="header-running">
    <span>Buku Panduan Penggunaan</span>
    <strong>Rajaku Printing</strong>
  </div>

  <div class="section-header" style="margin-top: 0;">
    <h1 class="section-title">Daftar Isi & Ikhtisar Sistem</h1>
    <span class="section-tag">Pengenalan</span>
  </div>

  <p style="font-size: 10pt; color: #44403C; margin-bottom: 16px;">
    <strong>Rajaku Printing</strong> adalah platform web manajemen percetakan banner terpadu. Sistem ini dirancang untuk menghilangkan kerumitan pesanan manual, mencegah salah cetak, dan memberikan pengalaman belanja yang transparan bagi pelanggan.
  </p>

  <div class="grid-2 avoid-break">
    <div class="card" style="border-left: 3px solid #8B1A1A;">
      <div class="card-header">📑 Ringkasan Bab Panduan</div>
      <ul style="padding-left: 16px; margin-bottom: 0; font-size: 8.5pt;">
        <li><strong>Bab 1:</strong> Panduan untuk Pelanggan (Order & Lacak Resi)</li>
        <li><strong>Bab 2:</strong> Panduan Kasir (POS Walk-in & Cetak Struk)</li>
        <li><strong>Bab 3:</strong> Panduan Verifikasi Pembayaran (Keuangan)</li>
        <li><strong>Bab 4:</strong> Panduan Tim Desainer (Cek File & Draft Desain)</li>
        <li><strong>Bab 5:</strong> Panduan Operator Produksi & QC Finishing</li>
        <li><strong>Bab 6:</strong> Panduan Admin & Pemilik Toko (Katalog & Staf)</li>
        <li><strong>Bab 7:</strong> Kamus Status Pesanan (State Machine)</li>
        <li><strong>Bab 8:</strong> Tanya Jawab (FAQ) & Solusi Kendala</li>
      </ul>
    </div>

    <div class="card" style="border-left: 3px solid #B08D57;">
      <div class="card-header">👥 6 Peran Utama Dalam Sistem</div>
      <table style="margin: 0; font-size: 7.5pt;">
        <tr>
          <td><span class="badge badge-crimson">Pelanggan</span></td>
          <td>Pesan online, cek estimasi harga, bayar, review desain.</td>
        </tr>
        <tr>
          <td><span class="badge badge-gold">Kasir (POS)</span></td>
          <td>Input pelanggan walk-in, terima uang, cetak nota thermal.</td>
        </tr>
        <tr>
          <td><span class="badge badge-green">Keuangan</span></td>
          <td>Cek mutasi bank/QRIS, approve bukti transfer.</td>
        </tr>
        <tr>
          <td><span class="badge badge-blue">Desainer</span></td>
          <td>Cek file cetak, buat draft banner, kelola revisi.</td>
        </tr>
        <tr>
          <td><span class="badge badge-gray">Produksi / QC</span></td>
          <td>Operasi mesin cetak, finishing mata ayam, packing.</td>
        </tr>
        <tr>
          <td><span class="badge badge-crimson">Super Admin</span></td>
          <td>Atur katalog, rekening, robot WA, akun staf & izin.</td>
        </tr>
      </table>
    </div>
  </div>

  <h2>Alur Kerja Pesanan Terpadu (End-to-End Workflow)</h2>
  <div class="step-flow avoid-break">
    <div class="step-node">
      <div class="step-num">1</div>
      <div class="step-text">Pemesanan</div>
      <div class="step-desc">Online / Kasir Walk-in</div>
    </div>
    <div class="step-node">
      <div class="step-num">2</div>
      <div class="step-text">Pembayaran</div>
      <div class="step-desc">Transfer / Cash / QRIS</div>
    </div>
    <div class="step-node gold">
      <div class="step-num">3</div>
      <div class="step-text">Desain / ACC</div>
      <div class="step-desc">Verifikasi File / Approval</div>
    </div>
    <div class="step-node gold">
      <div class="step-num">4</div>
      <div class="step-text">Cetak & QC</div>
      <div class="step-desc">Mesin Print + Finishing</div>
    </div>
    <div class="step-node dark">
      <div class="step-num">5</div>
      <div class="step-text">Selesai</div>
      <div class="step-desc">Ambil di Toko / Ekspedisi</div>
    </div>
  </div>

  <div class="callout tip avoid-break">
    <div class="callout-title">💡 Keunggulan Utama Rajaku Printing</div>
    Satu nomor WhatsApp menjadi <strong>kunci identitas tunggal</strong> pelanggan. Jika pelanggan pernah bertransaksi secara langsung di toko (walk-in) dan kemudian memesan lewat website dari rumah, seluruh riwayat pesanannya otomatis tersambung dalam satu akun.
  </div>
</div>

<!-- ========================================================== -->
<!-- 3. BAB 1: PANDUAN PELANGGAN -->
<!-- ========================================================== -->
<div class="doc-container page-break">
  <div class="header-running">
    <span>Bab 1: Panduan Pelanggan</span>
    <strong>Rajaku Printing</strong>
  </div>

  <div class="section-header" style="margin-top: 0;">
    <h1 class="section-title">1. Panduan untuk Pelanggan (Pemesanan & Pelacakan)</h1>
    <span class="section-tag">Jalur Pembeli</span>
  </div>

  <h2>A. Mengenal Bahan Banner & Karakteristiknya</h2>
  <table class="avoid-break">
    <thead>
      <tr>
        <th style="width: 25%;">Jenis Bahan</th>
        <th style="width: 35%;">Karakteristik & Ketebalan</th>
        <th style="width: 40%;">Rekomendasi Pemakaian</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td><strong>Flexi Standar</strong></td>
        <td>Ketebalan 280g – 340g, berserat halus, ekonomis.</td>
        <td>Spanduk acara singkat, promo toko bulanan, umbul-umbul.</td>
      </tr>
      <tr>
        <td><strong>Flexi Korea (Korcin)</strong></td>
        <td>Ketebalan 440g, permukaan doff halus, sangat ulet & tahan sobek.</td>
        <td>Baliho luar ruangan, plang toko permanen tahan hujan & panas.</td>
      </tr>
      <tr>
        <td><strong>Albatros / Luster</strong></td>
        <td>Halus tanpa serat, warna sangat tajam, tidak melengkung.</td>
        <td>X-Banner dan Roll-Up banner dalam ruangan (indoor/pameran).</td>
      </tr>
      <tr>
        <td><strong>Stiker Vinyl</strong></td>
        <td>Bahan plastik tahan air dengan lem rekat kuat.</td>
        <td>Stiker label etalase, branding kaca toko, tempelan neon box.</td>
      </tr>
    </tbody>
  </table>

  <h2>B. Langkah demi Langkah Pemesanan Online (Guest / Member)</h2>
  
  <div class="grid-2 avoid-break">
    <div class="card">
      <div class="card-header">Langkah 1: Tentukan Ukuran & Bahan</div>
      <p style="font-size: 8.5pt;">
        Buka menu <strong>Order Banner</strong> di website. Pilih jenis produk, pilih bahan, lalu masukkan panjang dan lebar dalam satuan sentimeter (cm). Harga total akan terhitung instan secara transparan.
      </p>
    </div>
    <div class="card">
      <div class="card-header">Langkah 2: Pilih Cara Pengambilan</div>
      <p style="font-size: 8.5pt;">
        <strong>Ambil Sendiri:</strong> Gratis ongkir, ambil langsung di toko jika sudah selesai.<br>
        <strong>Dikirim Kurir:</strong> Masukkan alamat rumah Anda, admin akan mengisi ongkir termurah.
      </p>
    </div>
  </div>

  <div class="grid-2 avoid-break">
    <div class="card">
      <div class="card-header">Langkah 3: Tentukan Opsi Desain</div>
      <p style="font-size: 8.5pt;">
        <strong>Punya Desain:</strong> Upload file CDR, AI, PDF, JPG, atau PNG siap cetak.<br>
        <strong>Minta Jasa Desain:</strong> Ketik tulisan banner yang diinginkan dan lampirkan foto logo/produk.
      </p>
    </div>
    <div class="card">
      <div class="card-header">Langkah 4: Masukkan No. WhatsApp</div>
      <p style="font-size: 8.5pt;">
        Ketik nama & nomor WhatsApp aktif Anda. Klik tombol <strong>Buat Pesanan</strong>. Anda akan langsung menerima <strong>Nomor Resi Unik</strong> (contoh: <code class="font-mono">RJK-8F3K2A9X</code>).
      </p>
    </div>
  </div>

  <h2>C. Cara Melacak Pesanan, Membayar, & Menyetujui Desain</h2>
  <div class="card avoid-break">
    <ol style="margin-bottom: 0; font-size: 8.5pt;">
      <li><strong>Cek Status Tanpa Login:</strong> Masuk ke menu <strong>Lacak Resi</strong>, masukkan nomor resi <code class="font-mono">RJK-XXXXXXXX</code>, lalu klik Lacak.</li>
      <li><strong>Verifikasi Pemilik:</strong> Masukkan nomor WhatsApp Anda untuk membuka rincian tagihan rekening dan tombol aksi.</li>
      <li><strong>Bayar & Upload Bukti:</strong> Transfer ke rekening bank toko atau scan QRIS yang tertera, foto struk transfer Anda, lalu klik tombol <strong>Unggah Bukti Bayar</strong>.</li>
      <li><strong>Persetujuan Desain (ACC):</strong> Jika Anda meminta jasa desain, desainer akan mengirim draft ke WA Anda. Buka halaman resi, klik <strong>"Setujui Desain (ACC)"</strong> jika sudah cocok, atau klik <strong>"Minta Revisi"</strong> jika ada koreksi.</li>
      <li><strong>Unduh Invoice PDF:</strong> Klik <strong>Download Invoice (PDF)</strong> untuk mencetak tanda terima resmi toko.</li>
    </ol>
  </div>
</div>

<!-- ========================================================== -->
<!-- 4. BAB 2 & 3: KASIR POS & VERIFIKASI PEMBAYARAN -->
<!-- ========================================================== -->
<div class="doc-container page-break">
  <div class="header-running">
    <span>Bab 2 & 3: Kasir POS & Keuangan</span>
    <strong>Rajaku Printing</strong>
  </div>

  <div class="section-header" style="margin-top: 0;">
    <h1 class="section-title">2. Panduan Kasir (POS / Walk-in)</h1>
    <span class="section-tag">Front Office</span>
  </div>

  <p style="font-size: 9pt; color: #44403C;">
    Menu <strong>POS (Point of Sale)</strong> dirancang untuk kecepatan operasional di meja kasir saat melayani pembeli yang datang langsung ke toko.
  </p>

  <div class="step-flow avoid-break">
    <div class="step-node">
      <div class="step-num">1</div>
      <div class="step-text">Input No. WA</div>
      <div class="step-desc">Auto-format 62xxx</div>
    </div>
    <div class="step-node">
      <div class="step-num">2</div>
      <div class="step-text">Pilih Ukuran</div>
      <div class="step-desc">Harga hitung live</div>
    </div>
    <div class="step-node gold">
      <div class="step-num">3</div>
      <div class="step-text">Bayar di Tempat</div>
      <div class="step-desc">Cash / QRIS Kasir</div>
    </div>
    <div class="step-node gold">
      <div class="step-num">4</div>
      <div class="step-text">Mode Desain</div>
      <div class="step-desc">ACC Langsung / Async</div>
    </div>
    <div class="step-node dark">
      <div class="step-num">5</div>
      <div class="step-text">Cetak Struk</div>
      <div class="step-desc">Thermal 58/80mm</div>
    </div>
  </div>

  <div class="grid-2 avoid-break">
    <div class="card">
      <div class="card-header">⚡ Resolusi Pelanggan Otomatis</div>
      <p style="font-size: 8pt; color: #57534E; margin-bottom: 0;">
        Kasir cukup memasukkan nomor HP pelanggan. Sistem akan mengecek apakah pelanggan sudah terdaftar sebelumnya. Jika pelanggan baru, sistem otomatis membuatkan data guest tanpa repot mengisi formulir pendaftaran yang panjang.
      </p>
    </div>
    <div class="card">
      <div class="card-header">🖨️ Cetak Struk Kasir Thermal</div>
      <p style="font-size: 8pt; color: #57534E; margin-bottom: 0;">
        Setelah tombol <strong>Buat Pesanan & Bayar</strong> diklik, popup cetak struk langsung muncul. Struk memuat nomor resi unik, rincian banner, dan total pembayaran. Ukuran kertas (58mm/80mm) disesuaikan otomatis dari setting admin.
      </p>
    </div>
  </div>

  <div class="section-header">
    <h1 class="section-title">3. Panduan Staf Keuangan (Verifikasi Pembayaran)</h1>
    <span class="section-tag">Admin Pembayaran</span>
  </div>

  <p style="font-size: 9pt; color: #44403C;">
    Menu <strong>Pembayaran</strong> digunakan untuk memvalidasi uang masuk dari pesanan online sebelum diteruskan ke proses pengerjaan desain dan cetak.
  </p>

  <table class="avoid-break">
    <thead>
      <tr>
        <th style="width: 25%;">Status / Tombol</th>
        <th style="width: 40%;">Tindakan Staf Keuangan</th>
        <th style="width: 35%;">Efek Sistem</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td><span class="badge badge-gold">Pending</span></td>
        <td>Buka menu Pembayaran ➔ Klik <strong>Lihat Bukti</strong>. Cocokkan nominal struk dengan mutasi rekening bank / QRIS toko.</td>
        <td>Pesanan menunggu validasi manual staf keuangan.</td>
      </tr>
      <tr>
        <td><span class="badge badge-green">Setujui (Approve)</span></td>
        <td>Klik tombol hijau jika uang sudah masuk di mutasi bank.</td>
        <td>Status berubah jadi <code class="font-mono">dibayar</code>, notifikasi WA terkirim, order maju ke antrean desainer.</td>
      </tr>
      <tr>
        <td><span class="badge badge-crimson">Tolak (Reject)</span></td>
        <td>Klik tombol merah jika struk palsu/kurang bayar. Ketik alasan penolakan secara jelas.</td>
        <td>Status kembali ke <code class="font-mono">menunggu_verifikasi</code>, pembeli menerima WA untuk upload ulang bukti baru.</td>
      </tr>
    </tbody>
  </table>
</div>

<!-- ========================================================== -->
<!-- 5. BAB 4 & 5: DESAINER & PRODUKSI -->
<!-- ========================================================== -->
<div class="doc-container page-break">
  <div class="header-running">
    <span>Bab 4 & 5: Desainer & Operator Produksi</span>
    <strong>Rajaku Printing</strong>
  </div>

  <div class="section-header" style="margin-top: 0;">
    <h1 class="section-title">4. Panduan Tim Desainer</h1>
    <span class="section-tag">Divisi Desain</span>
  </div>

  <div class="grid-3 avoid-break">
    <div class="card" style="border-top: 3px solid #8B1A1A;">
      <div class="card-header">Tab 1: Verifikasi Upload</div>
      <p style="font-size: 8pt; color: #57534E;">
        Untuk pelanggan yang membawa desain sendiri. Unduh file CDR/AI/PDF/JPG, cek ukuran dan ketajaman resolusi. Jika pas, klik <strong>Verifikasi & Lanjut Cetak</strong>.
      </p>
    </div>
    <div class="card" style="border-top: 3px solid #B08D57;">
      <div class="card-header">Tab 2: Kerjakan Request</div>
      <p style="font-size: 8pt; color: #57534E;">
        Untuk pesanan yang minta jasa desain. Baca brief teks & download aset logo. Buat desain, lalu upload gambar preview (JPG/PNG) dan kirim ke pelanggan via WA.
      </p>
    </div>
    <div class="card" style="border-top: 3px solid #171717;">
      <div class="card-header">Tab 3: Menunggu Approval</div>
      <p style="font-size: 8pt; color: #57534E;">
        Daftar draft yang sedang ditinjau pelanggan. Jika pelanggan klik ACC, order otomatis masuk antrean cetak. Jika minta revisi, perbaiki sesuai catatan.
      </p>
    </div>
  </div>

  <div class="callout tip avoid-break">
    <div class="callout-title">✨ Fitur Khusus: "Lewati Upload — Langsung Cetak" (Order Walk-in)</div>
    Jika pelanggan walk-in mengedit desain langsung di komputer kasir/desainer dan sudah setuju secara tatap muka, desainer tidak perlu mengunggah file besar ke sistem web. Cukup klik tombol <strong>"Lewati Upload — Langsung Cetak"</strong>, ketik catatan lokasi file di harddisk komputer (misal: <code class="font-mono">PC 1 / Spanduk-Bu-Sri.cdr</code>), dan order langsung masuk antrean cetak mesin.
  </div>

  <div class="section-header">
    <h1 class="section-title">5. Panduan Operator Produksi & QC</h1>
    <span class="section-tag">Workshop Percetakan</span>
  </div>

  <div class="step-flow avoid-break">
    <div class="step-node">
      <div class="step-num">1</div>
      <div class="step-text">Siap Cetak</div>
      <div class="step-desc">Download file / cek catatan</div>
    </div>
    <div class="step-node">
      <div class="step-num">2</div>
      <div class="step-text">Sedang Cetak</div>
      <div class="step-desc">Mesin print berjalan</div>
    </div>
    <div class="step-node gold">
      <div class="step-num">3</div>
      <div class="step-text">QC & Finishing</div>
      <div class="step-desc">Pasang ring mata ayam</div>
    </div>
    <div class="step-node gold">
      <div class="step-num">4</div>
      <div class="step-text">Siap Ambil/Kirim</div>
      <div class="step-desc">Kirim notif WA siap</div>
    </div>
    <div class="step-node dark">
      <div class="step-num">5</div>
      <div class="step-text">Selesai</div>
      <div class="step-desc">Diserahkan pembeli/kurir</div>
    </div>
  </div>

  <table class="avoid-break">
    <thead>
      <tr>
        <th style="width: 25%;">Tahap Produksi</th>
        <th style="width: 40%;">Aktivitas Fisik Operator</th>
        <th style="width: 35%;">Tombol Aksi di Sistem</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td><strong>Siap Cetak</strong></td>
        <td>Pasang gulungan bahan di mesin cetak, load file ke software RIP printer.</td>
        <td>Klik <strong>Mulai Cetak</strong> (status: <code class="font-mono">proses_cetak</code>).</td>
      </tr>
      <tr>
        <td><strong>Sedang Cetak</strong></td>
        <td>Pantau jalannya print mesin hingga selesai dan tinta mengering sempurna.</td>
        <td>Klik <strong>Kirim ke QC</strong> (status: <code class="font-mono">qc</code>).</td>
      </tr>
      <tr>
        <td><strong>QC & Finishing</strong></td>
        <td>Cek kecerahan warna, kerapian potong, dan kerjakan finishing (mata ayam/selongsong).</td>
        <td>Klik <strong>Mark Siap</strong> (status: <code class="font-mono">siap_ambil / siap_kirim</code>).</td>
      </tr>
      <tr>
        <td><strong>Pengambilan / Kirim</strong></td>
        <td>Serahkan spanduk ke pembeli di toko, atau serahkan ke kurir ekspedisi.</td>
        <td>Klik <strong>Mark Selesai</strong> atau <strong>Serahkan Kurir</strong> (isi no. resi kurir).</td>
      </tr>
    </tbody>
  </table>
</div>

<!-- ========================================================== -->
<!-- 6. BAB 6: ADMIN & PEMILIK TOKO -->
<!-- ========================================================== -->
<div class="doc-container page-break">
  <div class="header-running">
    <span>Bab 6: Manajemen & Super Admin</span>
    <strong>Rajaku Printing</strong>
  </div>

  <div class="section-header" style="margin-top: 0;">
    <h1 class="section-title">6. Panduan Admin & Pemilik Toko</h1>
    <span class="section-tag">Manajemen Sistem</span>
  </div>

  <div class="grid-2 avoid-break">
    <div class="card">
      <div class="card-header">📦 Manajemen Katalog & Harga</div>
      <p style="font-size: 8pt; color: #57534E;">
        Atur bahan dan produk di menu <strong>Katalog</strong>. Model harga fleksibel mendukung <strong>tipe per meter persegi ($m^2$)</strong> untuk banner custom, atau <strong>tipe paket</strong> untuk produk satuan fixed (X-Banner, Roll-Up).
      </p>
    </div>
    <div class="card">
      <div class="card-header">💳 Rekening & QRIS Dinamis</div>
      <p style="font-size: 8pt; color: #57534E;">
        Ubah nomor rekening bank di menu <strong>Pengaturan</strong>, dan unggah gambar QRIS toko di menu <strong>Media Landing Page</strong>. Perubahan langsung tampil di halaman checkout pelanggan tanpa perlu koding.
      </p>
    </div>
  </div>

  <div class="grid-2 avoid-break">
    <div class="card">
      <div class="card-header">📲 WhatsApp Gateway (Baileys)</div>
      <p style="font-size: 8pt; color: #57534E;">
        Buka menu <strong>WhatsApp</strong> di admin panel, lalu scan QR Code menggunakan aplikasi WhatsApp di HP toko Anda (menu <em>Perangkat Tertaut</em>). Begitu terhubung, seluruh notifikasi status dan link resi dikirim otomatis.
      </p>
    </div>
    <div class="card">
      <div class="card-header">🛡️ Kelola Staf & Hak Akses (Role)</div>
      <p style="font-size: 8pt; color: #57534E;">
        Tambah staf baru di menu <strong>Kelola Staff</strong> dengan menginput nama & nomor HP. Atur izin menu per karyawan di menu <strong>Role & Hak Akses</strong> (toggle izin: POS, Pembayaran, Desain, Produksi, atau Pengaturan).
      </p>
    </div>
  </div>

  <h2>Kebijakan Pembersihan Otomatis File Desain (Auto-Retention 30 Hari)</h2>
  <div class="card avoid-break" style="background: #FAFAF9;">
    <p style="font-size: 8.5pt; margin-bottom: 6px;">
      Untuk menjaga kapasitas harddisk server agar tidak penuh oleh file grafis berukuran ratusan Megabyte, sistem memiliki fitur <strong>Auto-Cleanup Retensi</strong>:
    </p>
    <ul style="font-size: 8pt; color: #44403C; margin-bottom: 0; padding-left: 16px;">
      <li>File fisik desain pelanggan yang sudah berusia <strong>30 hari</strong> sejak diunggah akan dihapus secara otomatis dari harddisk penyimpanan.</li>
      <li><strong>PENTING:</strong> Data transaksi tertulis, nomor resi, nama pelanggan, rincian biaya, dan invoice <strong>TIDAK PERNAH DIHAPUS</strong> dan tetap tersimpan aman selamanya untuk kebutuhan pembukuan.</li>
      <li>Masa retensi hari (default: 30 hari) dapat diubah kapan saja melalui menu <strong>Pengaturan</strong>.</li>
    </ul>
  </div>
</div>

<!-- ========================================================== -->
<!-- 7. BAB 7 & 8: KAMUS STATUS & FAQ -->
<!-- ========================================================== -->
<div class="doc-container page-break">
  <div class="header-running">
    <span>Bab 7 & 8: Kamus Status & FAQ</span>
    <strong>Rajaku Printing</strong>
  </div>

  <div class="section-header" style="margin-top: 0;">
    <h1 class="section-title">7. Kamus Status Pesanan (State Machine)</h1>
    <span class="section-tag">Referensi Status</span>
  </div>

  <table class="avoid-break" style="font-size: 8pt;">
    <thead>
      <tr>
        <th style="width: 28%;">Nama Status di Sistem</th>
        <th style="width: 42%;">Arti Sederhana dalam Bahasa Sehari-hari</th>
        <th style="width: 30%;">Siapa yang Bertindak?</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td><code class="font-mono">order_masuk</code></td>
        <td>Pesanan baru dibuat oleh pembeli melalui website.</td>
        <td>Admin (Cek ongkir jika kirim)</td>
      </tr>
      <tr>
        <td><code class="font-mono">menunggu_ongkir</code></td>
        <td>Pelanggan minta dikirim, menunggu admin input ongkir.</td>
        <td>Admin Keuangan / Ekspedisi</td>
      </tr>
      <tr>
        <td><code class="font-mono">menunggu_pembayaran</code></td>
        <td>Total biaya fix, pembeli diarahkan transfer ke rekening toko.</td>
        <td>Pelanggan (Lakukan transfer)</td>
      </tr>
      <tr>
        <td><code class="font-mono">menunggu_verifikasi</code></td>
        <td>Pelanggan sudah upload struk, menunggu divalidasi kasir.</td>
        <td>Staf Keuangan (Cek mutasi)</td>
      </tr>
      <tr>
        <td><code class="font-mono">dibayar</code></td>
        <td>Uang pembayaran sah telah diterima oleh toko.</td>
        <td>Desainer (Cek file / buat draft)</td>
      </tr>
      <tr>
        <td><code class="font-mono">desain_dikerjakan</code></td>
        <td>Desainer sedang menggambar banner sesuai permintaan pelanggan.</td>
        <td>Desainer</td>
      </tr>
      <tr>
        <td><code class="font-mono">menunggu_approval_desain</code></td>
        <td>Draft desain sudah dikirim ke WhatsApp pembeli.</td>
        <td>Pelanggan (Klik ACC / Revisi)</td>
      </tr>
      <tr>
        <td><code class="font-mono">desain_diverifikasi</code></td>
        <td>Desain sudah disetujui (ACC) dan siap masuk mesin cetak.</td>
        <td>Operator Produksi</td>
      </tr>
      <tr>
        <td><code class="font-mono">proses_cetak</code></td>
        <td>Spanduk sedang diprint oleh mesin cetak banner.</td>
        <td>Operator Produksi</td>
      </tr>
      <tr>
        <td><code class="font-mono">qc</code></td>
        <td>Pengecekan kualitas warna & proses finishing (mata ayam).</td>
        <td>Staf QC & Finishing</td>
      </tr>
      <tr>
        <td><code class="font-mono">siap_ambil</code> / <code class="font-mono">siap_kirim</code></td>
        <td>Spanduk sudah selesai dibungkus rapi, siap diserahkan.</td>
        <td>Kasir Toko / Kurir Ekspedisi</td>
      </tr>
      <tr>
        <td><code class="font-mono">dikirim</code></td>
        <td>Paket sedang dalam perjalanan kurir menuju alamat pembeli.</td>
        <td>Kurir Ekspedisi</td>
      </tr>
      <tr>
        <td><code class="font-mono">selesai</code></td>
        <td>Barang sudah diterima pembeli dengan baik. Transaksi sukses.</td>
        <td>Selesai</td>
      </tr>
    </tbody>
  </table>

  <div class="section-header">
    <h1 class="section-title">8. Tanya Jawab (FAQ) & Tips Solusi Kendala</h1>
    <span class="section-tag">Troubleshooting</span>
  </div>

  <div class="card avoid-break" style="margin-bottom: 8px;">
    <div class="card-header" style="font-size: 9pt;">❓ Apakah pelanggan wajib mendaftar akun untuk bisa memesan banner?</div>
    <p style="font-size: 8pt; color: #57534E; margin: 0;">
      <strong>Tidak wajib.</strong> Pelanggan dapat memesan sebagai Tamu (Guest) hanya dengan menginput Nama dan No. WhatsApp aktif. Seluruh update pesanan dan nota PDF dapat diakses langsung lewat nomor resi.
    </p>
  </div>

  <div class="card avoid-break" style="margin-bottom: 8px;">
    <div class="card-header" style="font-size: 9pt;">❓ Bagaimana jika bot WhatsApp toko terputus (Disconnected)?</div>
    <p style="font-size: 8pt; color: #57534E; margin: 0;">
      Buka menu <strong>Admin ➔ WhatsApp</strong>, klik <strong>Refresh / Tautkan Ulang</strong>, lalu scan ulang QR Code menggunakan WhatsApp HP toko Anda seperti biasa.
    </p>
  </div>

  <div class="card avoid-break" style="margin-bottom: 8px;">
    <div class="card-header" style="font-size: 9pt;">❓ Mengapa file CorelDraw (.CDR) tidak menampilkan gambar di browser?</div>
    <p style="font-size: 8pt; color: #57534E; margin: 0;">
      File CorelDraw (.CDR) dan Illustrator (.AI) adalah format file grafis mentah. Browser tidak bisa menampilkan preview-nya secara langsung. Desainer cukup klik tombol <strong>Unduh File</strong>, lalu membukanya di aplikasi CorelDraw / Illustrator pada komputer.
    </p>
  </div>

  <div class="card avoid-break" style="margin-bottom: 0;">
    <div class="card-header" style="font-size: 9pt;">❓ Bagaimana jika printer thermal kasir tidak merespon saat cetak struk?</div>
    <p style="font-size: 8pt; color: #57534E; margin: 0;">
      Pastikan kabel USB printer thermal terpasang dan printer dalam kondisi menyala. Pada jendela dialog cetak browser (<kbd>Ctrl + P</kbd>), pastikan kolom <em>Destination / Printer</em> memilih nama printer thermal Anda, bukan 'Save as PDF'.
    </p>
  </div>
</div>

</body>
</html>
"""

with open('d:/RAJAKU PRINTING/panduan_temp.html', 'w', encoding='utf-8') as f:
    f.write(html_content)

print("HTML generated successfully! Compiling to PDF via Chrome...")

chrome_path = r"C:\Program Files\Google\Chrome\Application\chrome.exe"
output_pdf = r"D:\RAJAKU PRINTING\PANDUAN_PENGGUNAAN_RAJAKU_PRINTING.pdf"
input_url = "file:///D:/RAJAKU PRINTING/panduan_temp.html"

cmd = [
    chrome_path,
    "--headless=new",
    "--disable-gpu",
    "--run-all-compositor-stages-before-draw",
    "--print-to-pdf-no-header",
    f"--print-to-pdf={output_pdf}",
    input_url
]

res = subprocess.run(cmd, capture_output=True, text=True)
print("Chrome return code:", res.returncode)
print("Chrome stdout:", res.stdout)
print("Chrome stderr:", res.stderr)

if os.path.exists(output_pdf):
    size_kb = os.path.getsize(output_pdf) / 1024
    print(f"SUCCESS: PDF created at '{output_pdf}' ({size_kb:.2f} KB)")
else:
    print("FAILED to create PDF")
